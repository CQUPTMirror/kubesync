package apiserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kubelog "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	defaultRetryPeriod = 2 * time.Second
	runLog             = kubelog.Log.WithName("kubesync").WithName("run")
)

type Options struct {
	Scheme    *runtime.Scheme
	Namespace string
	Port      int
}

type ApiServer interface {
	Start(ctx context.Context) error
}

type Manager struct {
	config     *rest.Config
	engine     *gin.Engine
	httpClient *http.Client
	client     client.Client
	started    bool
	internal   context.Context
	cache      cache.Cache
	port       int
	namespace  string
}

func GetTUNASyncManager(config *rest.Config, options Options) (ApiServer, error) {
	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		return nil, err
	}

	cc, err := cache.New(config, cache.Options{
		Scheme:    options.Scheme,
		Mapper:    mapper,
		Resync:    &defaultRetryPeriod,
		Namespace: options.Namespace,
	})
	if err != nil {
		return nil, err
	}

	c, err := client.New(config, client.Options{Scheme: options.Scheme, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	client, err := client.NewDelegatingClient(client.NewDelegatingClientInput{CacheReader: cc, Client: c})
	if err != nil {
		return nil, err
	}

	s := &Manager{
		config:    config,
		client:    client,
		internal:  context.Background(),
		cache:     cc,
		port:      options.Port,
		namespace: options.Namespace,
	}

	s.engine = gin.New()
	s.engine.Use(gin.Recovery())

	// common log middleware
	s.engine.Use(contextErrorLogger)

	s.engine.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{_infoKey: "pong"})
	})

	// list jobs, status page
	s.engine.GET("/jobs", s.listAllJobs)

	// mirror online
	s.engine.POST("/jobs", s.registerMirror)

	// mirrorID should be valid in this route group
	mirrorValidateGroup := s.engine.Group("/jobs")
	{
		// delete specified mirror
		mirrorValidateGroup.DELETE(":id", s.deleteJob)
		// get job list
		mirrorValidateGroup.GET(":id", s.getJob)
		// post job status
		mirrorValidateGroup.POST(":id", s.updateJob)
		mirrorValidateGroup.POST(":id/size", s.updateMirrorSize)
		mirrorValidateGroup.POST(":id/schedules", s.updateSchedules)
		// for tunasynctl to post commands
		mirrorValidateGroup.POST(":id/cmd", s.handleClientCmd)
	}

	return s, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.waitForCache()

	runLog.Info("Run tunasync manager server.")

	go func() {
		if err := m.Run(m.internal); err != nil {
			panic(err)
		}
	}()
	select {
	case <-ctx.Done():
		return nil
	}
}

func (m *Manager) waitForCache() {
	if m.started {
		return
	}

	go func() {
		if err := m.cache.Start(m.internal); err != nil {
			panic(err)
		}
	}()

	// Wait for the caches to sync.
	m.cache.WaitForCacheSync(m.internal)
	m.started = true
}
