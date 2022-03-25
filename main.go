package main

import (
	"context"
	_ "expvar"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/cyverse-de/configurate"
	"github.com/cyverse-de/dbutil"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

// IplantSuffix is what is appended to a username in the database.
const IplantSuffix = "@iplantcollaborative.org"

func jaegerTracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("user-info"),
		)),
	)

	return tp, nil
}

func main() {
	var (
		showVersion = flag.Bool("version", false, "Print the version information")
		cfgPath     = flag.String("config", "/etc/iplant/de/jobservices.yml", "The path to the config file")
		port        = flag.String("port", "60000", "The port number to listen on")
		err         error
		cfg         *viper.Viper

		tracerProvider *tracesdk.TracerProvider
	)

	flag.Parse()

	if *showVersion {
		AppVersion()
		os.Exit(0)
	}

	if *cfgPath == "" {
		log.Fatal("--config must be set")
	}

	otelTracesExporter := os.Getenv("OTEL_TRACES_EXPORTER")
	if otelTracesExporter == "jaeger" {
		jaegerEndpoint := os.Getenv("OTEL_EXPORTER_JAEGER_ENDPOINT")
		if jaegerEndpoint == "" {
			log.Warn("Jaeger set as OpenTelemetry trace exporter, but no Jaeger endpoint configured.")
		} else {
			tp, err := jaegerTracerProvider(jaegerEndpoint)
			if err != nil {
				log.Fatal(err)
			}
			tracerProvider = tp
			otel.SetTracerProvider(tp)
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
		}
	}

	if tracerProvider != nil {
		tracerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		defer func(tracerContext context.Context) {
			ctx, cancel := context.WithTimeout(tracerContext, time.Second*5)
			defer cancel()
			if err := tracerProvider.Shutdown(ctx); err != nil {
				log.Fatal(err)
			}
		}(tracerCtx)
	}

	if cfg, err = configurate.InitDefaults(*cfgPath, configurate.JobServicesDefaults); err != nil {
		log.Fatal(err.Error())
	}

	dburi := cfg.GetString("db.uri")
	connector, err := dbutil.NewDefaultConnector("1m")
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Info("Connecting to the database...")
	db, err := connector.Connect("postgres", dburi)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer db.Close()
	log.Info("Connected to the database.")

	if err := db.Ping(); err != nil {
		log.Fatal(err.Error())
	}
	log.Info("Successfully pinged the database")

	userDomain := cfg.GetString("users.domain")
	if userDomain == "" {
		userDomain = IplantSuffix
	}

	router := makeRouter()

	prefsDB := NewPrefsDB(db)
	prefsApp := NewPrefsApp(prefsDB, router)

	sessionsDB := NewSessionsDB(db)
	sessionsApp := NewSessionsApp(sessionsDB, router)

	searchesDB := NewSearchesDB(db)
	searchesApp := NewSearchesApp(searchesDB, router)

	bagsApp := NewBagsApp(db, router, userDomain)

	log.Debug(prefsApp)
	log.Debug(sessionsApp)
	log.Debug(searchesApp)
	log.Debug(bagsApp)

	log.Info("Listening on port ", *port)
	log.Fatal(http.ListenAndServe(fixAddr(*port), router))
}
