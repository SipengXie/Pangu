package node

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/SipengXie/pangu/node/nodecfg"
	"github.com/c2h5oh/datasize"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/gofrs/flock"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon-lib/kv/memdb"
	"github.com/ledgerwatch/log/v3"
	"golang.org/x/sync/semaphore"
)

const (
	initializingState = iota
	runningState
	closedState
)

// Node is a container on which services can be registered.
type Node struct {
	config        *nodecfg.Config
	logger        log.Logger
	dirLock       *flock.Flock  // 防止与其他实例目录并发使用
	stop          chan struct{} // Channel to wait for termination notifications
	startStopLock sync.Mutex    // Node的附加锁，用于保护Node启动与关闭
	state         int           // Tracks state of node lifecycle

	lock       sync.Mutex
	lifecycles []Lifecycle // All registered backends, services, and auxiliary services that have a lifecycle

	databases []kv.Closer
}

// New 新建一个P2P节点, ready for protocol registration.
func New(conf *nodecfg.Config, logger log.Logger) (*Node, error) {
	// 准备配置文件
	confCopy := *conf
	conf = &confCopy

	// 对配置文件进行简单检查
	if strings.ContainsAny(conf.Name, `/\`) {
		return nil, errors.New(`Config.Name must not contain '/' or '\'`)
	}
	if strings.HasSuffix(conf.Name, ".ipc") {
		return nil, errors.New(`Config.Name cannot end in ".ipc"`)
	}

	node := &Node{
		config:    conf,
		logger:    logger,
		stop:      make(chan struct{}),
		databases: make([]kv.Closer, 0),
	}

	// 打开节点数据路径，需要实例化目录锁
	if err := node.OpenDataDir(); err != nil {
		return nil, err
	}

	return node, nil
}

// Start starts all registered lifecycles, RPC services and p2p networking.
// Node 只能被启动一次
func (n *Node) Start() error {
	// 启动时进行附加锁的锁定操作
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	// 开启节点状态修改锁
	n.lock.Lock()
	switch n.state {
	case runningState:
		n.lock.Unlock()
		return ErrNodeRunning
	case closedState:
		n.lock.Unlock()
		return ErrNodeStopped
	}
	n.state = runningState // 由初始化状态切换到运行状态
	lifecycles := make([]Lifecycle, len(n.lifecycles))
	copy(lifecycles, n.lifecycles) // 拷贝节点上注册的服务
	n.lock.Unlock()

	// 启动所有以注册的服务（lifecycle）
	// ! 预分配会导致此处的错误
	var started []Lifecycle //nolint:prealloc
	var err error
	for _, lifecycle := range lifecycles {
		// 逐一启动服务
		if err = lifecycle.Start(); err != nil {
			break
		}
		started = append(started, lifecycle)
	}

	// 启动时错误检查
	if err != nil {
		// 将已启动的服务关闭
		stopErr := n.stopServices(started)
		if stopErr != nil {
			n.logger.Warn("Failed to doClose for this node", "err", stopErr)
		} //nolint:errcheck
		// 关闭节点
		closeErr := n.doClose(nil)
		if closeErr != nil {
			n.logger.Warn("Failed to doClose for this node", "err", closeErr)
		}
	}
	return err
}

// Close 关闭Node并且释放资源
func (n *Node) Close() error {
	// 关闭时进行附加锁的锁定操作
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	// 开启节点状态修改锁
	n.lock.Lock()
	state := n.state
	n.lock.Unlock()
	switch state {
	case initializingState:
		// 这种情况是Node还没有被启动
		return n.doClose(nil)
	case runningState:
		// 这种情况是Node已经启动, 需要释放Start()获取的资源.
		var errs []error
		if err := n.stopServices(n.lifecycles); err != nil {
			errs = append(errs, err)
		}
		return n.doClose(errs)
	case closedState:
		return ErrNodeStopped
	default:
		panic(fmt.Sprintf("node is in unknown state %d", state))
	}
}

// doClose 释放 New() 获取的资源并收集错误。
func (n *Node) doClose(errs []error) error {
	// 关闭数据库，需要锁来防止对数据库的并发打开/关闭
	n.lock.Lock()
	n.state = closedState
	for _, closer := range n.databases {
		closer.Close()
	}
	n.lock.Unlock()

	// 释放实例目录锁
	n.closeDataDir()

	// Unblock n.Wait.
	close(n.stop)

	// 报告可能发生的任何错误。
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%v", errs)
	}
}

// stopService 关闭已启动的服务、RPC 以及 P2P 网络
func (n *Node) stopServices(running []Lifecycle) error {
	// 关闭正在运行的Lifecycles
	failure := &StopError{Services: make(map[reflect.Type]error)}
	for i := len(running) - 1; i >= 0; i-- {
		if err := running[i].Stop(); err != nil {
			failure.Services[reflect.TypeOf(running[i])] = err
		}
	}
	if len(failure.Services) > 0 {
		return failure
	}
	// 正常关闭
	return nil
}

func StartNode(stack *Node) {
	if err := stack.Start(); err != nil {
		utils.Fatalf("Error starting protocol stack: %v", err)
	}

	// go debug.ListenSignals(stack, stack.logger)
}

func (n *Node) OpenDataDir() error {
	if n.config.Dirs.DataDir == "" {
		return nil
	} // ! 可能有问题

	instanceDir := n.config.Dirs.DataDir
	if err := os.Mkdir(instanceDir, 0700); err != nil {
		return err
	}

	//锁定实例目录以防止其他实例并发使用，以及意外使用实例目录作为数据库。
	l := flock.New(filepath.Join(instanceDir, "LOCK"))

	// 启动一个读写锁
	locked, err := l.TryLock()
	if err != nil {
		return convertFileLockError(err)
	}
	if !locked {
		return fmt.Errorf("%w: %s", ErrDataDirUsed, instanceDir)
	}
	n.dirLock = l
	return nil
}

func (n *Node) closeDataDir() {
	// 释放实例目录锁
	if n.dirLock != nil {
		if err := n.dirLock.Unlock(); err != nil {
			n.logger.Error("Can't release datadir lock", "err", err)
		}
		n.dirLock = nil
	}
}

// containsLifecycle checks if 'lfs' contains 'l'.
func containsLifecycle(lfs []Lifecycle, l Lifecycle) bool {
	for _, obj := range lfs {
		if obj == l {
			return true
		}
	}
	return false
}

// Wait blocks until the node is closed.
func (n *Node) Wait() {
	<-n.stop
}

// RegisterLifecycle 将给定的lifecycle注册到node中
func (n *Node) RegisterLifecycle(lifecycle Lifecycle) {
	n.lock.Lock()
	defer n.lock.Unlock()

	// 仅在初始化状态阶段可注册服务
	if n.state != initializingState {
		panic("can't register lifecycle on running/stopped node")
	}
	if containsLifecycle(n.lifecycles, lifecycle) {
		panic(fmt.Sprintf("attempt to register lifecycle %T more than once", lifecycle))
	}
	n.lifecycles = append(n.lifecycles, lifecycle)
}

// Config returns the configuration of node.
func (n *Node) Config() *nodecfg.Config {
	return n.config
}

// DataDir retrieves the current datadir used by the protocol stack.
func (n *Node) DataDir() string {
	return n.config.Dirs.DataDir
}

func OpenDatabase(config *nodecfg.Config, label kv.Label, name string, readonly bool, logger log.Logger) (kv.RwDB, error) {
	switch label {
	case kv.ChainDB:
		name = "chaindata"
	case kv.TxPoolDB:
		name = "txpool"
	case kv.ConsensusDB:
		if len(name) == 0 {
			return nil, fmt.Errorf("Expected a consensus name")
		}
	default:
		name = "test"
	}
	var db kv.RwDB
	// 若路径为空则打开一个内存数据库
	if config.Dirs.DataDir == "" {
		db = memdb.New("")
		return db, nil
	}

	dbPath := filepath.Join(config.Dirs.DataDir, name)
	logger.Info("Opening Database", "label", name, "path", dbPath)
	openFunc := func(exclusive bool) (kv.RwDB, error) {
		roTxLimit := int64(32)
		// if config.Http.DBReadConcurrency > 0 {
		// 	roTxLimit = int64(config.Http.DBReadConcurrency)
		// }
		roTxsLimiter := semaphore.NewWeighted(roTxLimit) // 1 less than max to allow unlocking to happen
		opts := mdbx.NewMDBX(log.Root()).
			Path(dbPath).Label(label).
			DBVerbosity(config.DatabaseVerbosity).RoTxsLimiter(roTxsLimiter)

		if readonly {
			opts = opts.Readonly()
		}
		if exclusive {
			opts = opts.Exclusive()
		}

		switch label {
		case kv.ChainDB, kv.ConsensusDB:
			if config.MdbxPageSize.Bytes() > 0 {
				opts = opts.PageSize(config.MdbxPageSize.Bytes())
			}
			if config.MdbxDBSizeLimit > 0 {
				opts = opts.MapSize(config.MdbxDBSizeLimit)
			}
			if config.MdbxGrowthStep > 0 {
				opts = opts.GrowthStep(config.MdbxGrowthStep)
			}
		default:
			opts = opts.GrowthStep(16 * datasize.MB)
		}

		return opts.Open()
	}
	var err error
	db, err = openFunc(false)
	if err != nil {
		return nil, err
	}
	// TODO : Migration 逻辑待补充
	return db, nil
}
