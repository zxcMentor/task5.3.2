package main

import (
	"fmt"
	"geotask_pprof/module/courier/models"
	_ "geotask_pprof/module/courier/models"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"
	"os"
	"time"
)

//	func main() {
//		godotenv.Load()
//		// инициализация приложения
//		app := run.NewApp()
//		// запуск приложения
//		err := app.Run()
//		// в случае ошибки выводим ее в лог и завершаем работу с кодом 2
//		if err != nil {
//			log.Println(fmt.Sprintf("error: %s", err))
//			os.Exit(2)
//		}
//	}
const (
	redisKeyCourier = "courier"
	redisKeyOrders  = "orders"
	CourierRadius   = 2500
)

var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	requestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_total",
			Help: "Request total",
		},
		[]string{"endpoint"},
	)

	cacheDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cache_duration_seconds",
			Help:    "Cache duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	dbDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_duration_seconds",
			Help:    "DB duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	externalAPIDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "external_api_duration_seconds",
			Help:    "API duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")

		if token != "some token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
func pprofHandler(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	profileType := r.URL.Path[len("/mycustompath/pprof/"):]
	switch profileType {
	case "allocs", "block", "cmdline", "goroutine", "heap", "mutex", "profile", "threadcreate", "trace":
		pprof.Index(w, r)
	default:
		http.NotFound(w, r)
	}
}
func isAuthorized(r *http.Request) bool {
	token := r.Header.Get("Authorization")
	return token == "some token"
}
func saveProfile(endpoint, filename string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:6060%s", endpoint), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

// @title courier service
// @version 1.0
// @description courier service
// @host localhost:8080
// @BasePath /api/v1
//
//go:generate swagger generate spec -o ../public/swagger.json --scan-models
func main() {
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestCount)
	prometheus.MustRegister(cacheDuration)
	prometheus.MustRegister(dbDuration)
	prometheus.MustRegister(externalAPIDuration)

	http.Handle("/metrics", promhttp.Handler())

	log.Println(http.ListenAndServe("localhost:9090", nil))
	go func() {
		http.Handle("/mycustompath/pprof/", http.HandlerFunc(pprofHandler))
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	router := gin.Default()

	http.Handle("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
	))

	router.POST("/move-courier", func(c *gin.Context) {
		startTime := time.Now()
		var courierLocation models.Point
		if err := c.BindJSON(&courierLocation); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Courier location updated"})
		duration := time.Since(startTime).Seconds()

		requestDuration.WithLabelValues("/move-courier").Observe(duration)
		requestCount.WithLabelValues("/move-courier").Inc()
	})

	router.Run(":8080")
	if err := saveProfile("/mycustompath/pprof/profile", "profile.pprof"); err != nil {
		fmt.Println("Error saving profile:", err)
	}
}
