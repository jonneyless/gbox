package rabbitmq

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonneyless/gbox/logger"

	"github.com/wagslane/go-rabbitmq"
	"go.uber.org/zap"
)

const (
	defaultConnectionTimeout = 5 * time.Second
	defaultIdleTimeout       = 5 * time.Minute
	defaultCleanupInterval   = 5 * time.Minute
	defaultWaitTimeout       = 5 * time.Second
	defaultCloseTimeout      = 10 * time.Second
)

var (
	rabbitMQ   *RabbitMQ
	rabbitOnce sync.Once
)

type RabbitParams struct {
	Scheme      string
	Host        string
	Port        int
	Username    string
	Password    string
	VirtualHost string
}

type PoolConfig struct {
	MaxConnections     int
	MaxIdleConnections int
	Heartbeat          time.Duration
	ConnectionTimeout  time.Duration
	IdleTimeout        time.Duration
}

func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConnections:     20,
		MaxIdleConnections: 10,
		Heartbeat:          20 * time.Second,
		ConnectionTimeout:  defaultConnectionTimeout,
		IdleTimeout:        defaultIdleTimeout,
	}
}

type ConnectionWrapper struct {
	conn        *rabbitmq.Conn
	lastUsed    time.Time
	mu          sync.RWMutex
	log         *zap.SugaredLogger
	publisherMu sync.Mutex
	publishers  map[string]*rabbitmq.Publisher
}

func (w *ConnectionWrapper) getPublisher(exchange string) (*rabbitmq.Publisher, error) {
	w.publisherMu.Lock()
	defer w.publisherMu.Unlock()

	if pub, exists := w.publishers[exchange]; exists {
		return pub, nil
	}

	pub, err := rabbitmq.NewPublisher(
		w.conn,
		rabbitmq.WithPublisherOptionsLogger(w.log),
		rabbitmq.WithPublisherOptionsExchangeName(exchange),
		rabbitmq.WithPublisherOptionsExchangeDeclare,
		rabbitmq.WithPublisherOptionsExchangeKind("direct"),
	)
	if err != nil {
		return nil, err
	}
	w.publishers[exchange] = pub
	return pub, nil
}

func (w *ConnectionWrapper) GetConn() *rabbitmq.Conn {
	return w.conn
}

func (w *ConnectionWrapper) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.publisherMu.Lock()
	for _, publisher := range w.publishers {
		if publisher != nil {
			publisher.Close()
		}
	}
	w.publishers = make(map[string]*rabbitmq.Publisher)
	w.publisherMu.Unlock()

	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

type ConnectionPool struct {
	dsn    string
	config *PoolConfig
	idle   chan *ConnectionWrapper
	closed atomic.Bool
	logger *zap.SugaredLogger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	stats  PoolStats
}

type PoolStats struct {
	TotalConnections  int32
	ActiveConnections int32
	IdleConnections   int32
}

func (p *ConnectionPool) createConnection() (*ConnectionWrapper, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("connection pool is closed")
	}

	amqpConfig := rabbitmq.Config{
		Heartbeat: p.config.Heartbeat,
	}

	conn, err := rabbitmq.NewConn(
		p.dsn,
		rabbitmq.WithConnectionOptionsLogger(p.logger),
		rabbitmq.WithConnectionOptionsConfig(amqpConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	return &ConnectionWrapper{
		conn:       conn,
		lastUsed:   time.Now(),
		log:        p.logger,
		publishers: make(map[string]*rabbitmq.Publisher),
	}, nil
}

func (p *ConnectionPool) getConnection() (*ConnectionWrapper, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("connection pool is closed")
	}

	select {
	case conn := <-p.idle:
		p.updateStatsOnGet(conn)
		return conn, nil
	default:
	}

	for {
		current := atomic.LoadInt32(&p.stats.TotalConnections)
		if current >= int32(p.config.MaxConnections) {
			return p.waitForIdleConnection()
		}

		if atomic.CompareAndSwapInt32(&p.stats.TotalConnections, current, current+1) {
			conn, err := p.createConnection()
			if err != nil {
				atomic.AddInt32(&p.stats.TotalConnections, -1)
				return nil, err
			}
			atomic.AddInt32(&p.stats.ActiveConnections, 1)
			return conn, nil
		}
	}
}

func (p *ConnectionPool) waitForIdleConnection() (*ConnectionWrapper, error) {
	select {
	case conn := <-p.idle:
		p.updateStatsOnGet(conn)
		return conn, nil
	case <-time.After(defaultWaitTimeout):
		return nil, fmt.Errorf("timeout waiting for available connection")
	}
}

func (p *ConnectionPool) updateStatsOnGet(conn *ConnectionWrapper) {
	atomic.AddInt32(&p.stats.ActiveConnections, 1)
	atomic.AddInt32(&p.stats.IdleConnections, -1)
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.mu.Unlock()
}

func (p *ConnectionPool) returnConnection(wrapper *ConnectionWrapper) {
	if wrapper == nil || p.closed.Load() {
		return
	}

	wrapper.mu.Lock()
	wrapper.lastUsed = time.Now()
	wrapper.mu.Unlock()

	select {
	case p.idle <- wrapper:
		atomic.AddInt32(&p.stats.ActiveConnections, -1)
		atomic.AddInt32(&p.stats.IdleConnections, 1)
	default:
		_ = wrapper.Close()
		atomic.AddInt32(&p.stats.TotalConnections, -1)
		atomic.AddInt32(&p.stats.ActiveConnections, -1)
	}
}

func (p *ConnectionPool) cleanup() {
	now := time.Now()
	idleTimeout := p.config.IdleTimeout

	for i := 0; i < len(p.idle); i++ {
		select {
		case conn := <-p.idle:
			conn.mu.RLock()
			lastUsed := conn.lastUsed
			conn.mu.RUnlock()

			if now.Sub(lastUsed) > idleTimeout {
				_ = conn.Close()
				atomic.AddInt32(&p.stats.TotalConnections, -1)
				atomic.AddInt32(&p.stats.IdleConnections, -1)
			} else {
				select {
				case p.idle <- conn:
				default:
					_ = conn.Close()
					atomic.AddInt32(&p.stats.TotalConnections, -1)
				}
			}
		default:
			return
		}
	}
}

func (p *ConnectionPool) cleanupIdleConnections() {
	defer p.wg.Done()

	ticker := time.NewTicker(defaultCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *ConnectionPool) HealthCheck() error {
	if p.closed.Load() {
		return fmt.Errorf("connection pool is closed")
	}

	select {
	case conn := <-p.idle:
		p.returnConnection(conn)
		return nil
	default:
		if atomic.LoadInt32(&p.stats.TotalConnections) < int32(p.config.MaxConnections) {
			conn, err := p.createConnection()
			if err != nil {
				return err
			}
			p.returnConnection(conn)
			return nil
		}
		return fmt.Errorf("no available connections")
	}
}

type Publisher struct {
	pool    *ConnectionPool
	wrapper *ConnectionWrapper
	closed  bool
	mu      sync.Mutex
	log     *zap.SugaredLogger
}

func (p *Publisher) Publish(exchange string, routingKeys []string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	pub, err := p.wrapper.getPublisher(exchange)
	if err != nil {
		return err
	}

	return pub.Publish(
		body,
		routingKeys,
		rabbitmq.WithPublishOptionsContentType("application/json"),
		rabbitmq.WithPublishOptionsExchange(exchange),
	)
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	p.pool.returnConnection(p.wrapper)
	return nil
}

type Consumer struct {
	consumer *rabbitmq.Consumer
	pool     *ConnectionPool
	wrapper  *ConnectionWrapper
	queue    string
	closed   bool
	mu       sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	returned bool
}

func (c *Consumer) GetConnection() *rabbitmq.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wrapper.conn
}

func (c *Consumer) Consume(ctx context.Context, handler func(d rabbitmq.Delivery) rabbitmq.Action) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("consumer is closed")
	}

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.mu.Unlock()

	errChan := make(chan error, 1)

	go func() {
		c.wg.Add(1)
		defer c.wg.Done()
		errChan <- c.consumer.Run(handler)
	}()

	go func() {
		<-ctx.Done()
		c.consumer.Close()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Consumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	if c.cancel != nil {
		c.cancel()
	}

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultCloseTimeout):
		if c.pool.logger != nil {
			c.pool.logger.Warn("Consumer close timeout")
		}
	}

	if c.consumer != nil {
		c.consumer.Close()
	}

	if c.wrapper != nil && !c.returned {
		c.returned = true
		c.pool.returnConnection(c.wrapper)
	}

	return nil
}

type RabbitMQ struct {
	pool      *ConnectionPool
	publisher *Publisher
	pubMu     sync.Mutex
}

func (r *RabbitMQ) GetPublisher() (*Publisher, error) {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	// 如果已存在且未关闭，直接返回
	if r.publisher != nil && !r.publisher.closed {
		return r.publisher, nil
	}

	// 创建新的发布者
	wrapper, err := r.pool.getConnection()
	if err != nil {
		return nil, err
	}

	r.publisher = &Publisher{
		pool:    r.pool,
		wrapper: wrapper,
		closed:  false,
		log:     r.pool.logger,
	}
	return r.publisher, nil
}

func (r *RabbitMQ) ClosePublisher() error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	if r.publisher != nil && !r.publisher.closed {
		return r.publisher.Close()
	}
	return nil
}

func (r *RabbitMQ) GetConsumer(exchange string, queue string, opts ...func(*rabbitmq.ConsumerOptions)) (*Consumer, error) {
	if r.pool == nil || r.pool.closed.Load() {
		return nil, fmt.Errorf("RabbitMQ not initialized or closed")
	}

	wrapper, err := r.pool.getConnection()
	if err != nil {
		return nil, err
	}

	baseOpts := []func(*rabbitmq.ConsumerOptions){
		rabbitmq.WithConsumerOptionsLogger(r.pool.logger),
		rabbitmq.WithConsumerOptionsRoutingKey(queue),
		rabbitmq.WithConsumerOptionsExchangeName(exchange),
		rabbitmq.WithConsumerOptionsExchangeDeclare,
		rabbitmq.WithConsumerOptionsQueueDurable,
	}

	allOpts := append(baseOpts, opts...)

	consumer, err := rabbitmq.NewConsumer(
		wrapper.conn,
		queue,
		allOpts...,
	)
	if err != nil {
		r.pool.returnConnection(wrapper)
		return nil, fmt.Errorf("create consumer failed: %w", err)
	}

	return &Consumer{
		consumer: consumer,
		pool:     r.pool,
		wrapper:  wrapper,
		queue:    queue,
	}, nil
}

func (r *RabbitMQ) GetStats() PoolStats {
	return PoolStats{
		TotalConnections:  atomic.LoadInt32(&r.pool.stats.TotalConnections),
		ActiveConnections: atomic.LoadInt32(&r.pool.stats.ActiveConnections),
		IdleConnections:   atomic.LoadInt32(&r.pool.stats.IdleConnections),
	}
}

func (r *RabbitMQ) HealthCheck() error {
	return r.pool.HealthCheck()
}

func (r *RabbitMQ) Close() error {
	if r.pool == nil {
		return nil
	}

	r.pool.closed.Store(true)
	r.pool.cancel()
	r.pool.wg.Wait()

	close(r.pool.idle)
	for conn := range r.pool.idle {
		_ = conn.Close()
	}
	return nil
}

func InitRabbit(params *RabbitParams) {
	InitRabbitWithConfig(params, DefaultPoolConfig())
}

func buildAMQPURL(scheme, username, password, host string, port int, vhost string) string {
	if scheme == "" {
		scheme = "amqp"
	}

	u := url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, port),
		User:   url.UserPassword(username, password),
		Path:   vhost,
	}
	return u.String()
}

func InitRabbitWithConfig(params *RabbitParams, config *PoolConfig) {
	rabbitOnce.Do(func() {
		dsn := buildAMQPURL(
			params.Scheme,
			params.Username,
			params.Password,
			params.Host,
			params.Port,
			params.VirtualHost,
		)

		ctx, cancel := context.WithCancel(context.Background())

		pool := &ConnectionPool{
			dsn:    dsn,
			config: config,
			idle:   make(chan *ConnectionWrapper, config.MaxIdleConnections),
			logger: logger.GetLogger(),
			ctx:    ctx,
			cancel: cancel,
		}

		for i := 0; i < config.MaxIdleConnections; i++ {
			conn, err := pool.createConnection()
			if err != nil {
				pool.logger.Warnw("Failed to create initial connection", "error", err)
				continue
			}
			pool.idle <- conn
			atomic.AddInt32(&pool.stats.TotalConnections, 1)
			atomic.AddInt32(&pool.stats.IdleConnections, 1)
		}

		pool.wg.Add(1)
		go pool.cleanupIdleConnections()

		rabbitMQ = &RabbitMQ{pool: pool}
	})
}

func GetRabbit() (*RabbitMQ, error) {
	if rabbitMQ == nil {
		return nil, fmt.Errorf("RabbitMQ not initialized")
	}
	return rabbitMQ, nil
}
