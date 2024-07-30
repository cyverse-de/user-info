package main

import (
	"context"
	_ "expvar"
	"flag"
	"net/http"
	"os"
	"strings"

	"github.com/cyverse-de/configurate"
	"github.com/cyverse-de/dbutil"
	"github.com/cyverse-de/go-mod/otelutils"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const serviceName = "user-info"

// IplantSuffix is what is appended to a username in the database.
const IplantSuffix = "iplantcollaborative.org"

func main() {
	var (
		showVersion = flag.Bool("version", false, "Print the version information")
		cfgPath     = flag.String("config", "/etc/iplant/de/jobservices.yml", "The path to the config file")
		port        = flag.String("port", "60000", "The port number to listen on")
		err         error
		cfg         *viper.Viper
	)

	flag.Parse()

	if *showVersion {
		AppVersion()
		os.Exit(0)
	}

	if *cfgPath == "" {
		log.Fatal("--config must be set")
	}

	var tracerCtx, cancel = context.WithCancel(context.Background())
	defer cancel()
	shutdown := otelutils.TracerProviderFromEnv(tracerCtx, serviceName, func(e error) { log.Fatal(e) })
	defer shutdown()

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

	userDomain := strings.Trim(cfg.GetString("users.domain"), "@")
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
