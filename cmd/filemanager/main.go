package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/asdine/storm"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/hacdias/filemanager"
	"github.com/hacdias/filemanager/bolt"
	h "github.com/hacdias/filemanager/http"
	"github.com/hacdias/filemanager/staticgen"
	"github.com/hacdias/fileutils"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	addr          string
	config        string
	database      string
	scope         string
	commands      string
	logfile       string
	staticg       string
	locale        string
	baseurl       string
	prefixurl     string
	port          int
	noAuth        bool
	allowCommands bool
	allowEdit     bool
	allowNew      bool
	allowPublish  bool
	showVer       bool
)

func init() {
	flag.StringVarP(&config, "config", "c", "", "Configuration file")
	flag.IntVarP(&port, "port", "p", 0, "HTTP Port (default is random)")
	flag.StringVarP(&addr, "address", "a", "", "Address to listen to (default is all of them)")
	flag.StringVarP(&database, "database", "d", "./filemanager.db", "Database file")
	flag.StringVarP(&logfile, "log", "l", "stdout", "Errors logger; can use 'stdout', 'stderr' or file")
	flag.StringVarP(&scope, "scope", "s", ".", "Default scope option for new users")
	flag.StringVarP(&baseurl, "baseurl", "b", "", "Base URL")
	flag.StringVar(&commands, "commands", "git svn hg", "Default commands option for new users")
	flag.StringVar(&prefixurl, "prefixurl", "", "Prefix URL")
	flag.BoolVar(&allowCommands, "allow-commands", true, "Default allow commands option for new users")
	flag.BoolVar(&allowEdit, "allow-edit", true, "Default allow edit option for new users")
	flag.BoolVar(&allowPublish, "allow-publish", true, "Default allow publish option for new users")
	flag.BoolVar(&allowNew, "allow-new", true, "Default allow new option for new users")
	flag.BoolVar(&noAuth, "no-auth", false, "Disables authentication")
	flag.StringVar(&locale, "locale", "en", "Default locale for new users")
	flag.StringVar(&staticg, "staticgen", "", "Static Generator you want to enable")
	flag.BoolVarP(&showVer, "version", "v", false, "Show version")
}

func setupViper() {
	viper.SetDefault("Address", "")
	viper.SetDefault("Port", "0")
	viper.SetDefault("Database", "./filemanager.db")
	viper.SetDefault("Scope", ".")
	viper.SetDefault("Logger", "stdout")
	viper.SetDefault("Commands", []string{"git", "svn", "hg"})
	viper.SetDefault("AllowCommmands", true)
	viper.SetDefault("AllowEdit", true)
	viper.SetDefault("AllowNew", true)
	viper.SetDefault("AllowPublish", true)
	viper.SetDefault("StaticGen", "")
	viper.SetDefault("Locale", "en")
	viper.SetDefault("NoAuth", false)
	viper.SetDefault("BaseURL", "")
	viper.SetDefault("PrefixURL", "")

	viper.BindPFlag("Port", flag.Lookup("port"))
	viper.BindPFlag("Address", flag.Lookup("address"))
	viper.BindPFlag("Database", flag.Lookup("database"))
	viper.BindPFlag("Scope", flag.Lookup("scope"))
	viper.BindPFlag("Logger", flag.Lookup("log"))
	viper.BindPFlag("Commands", flag.Lookup("commands"))
	viper.BindPFlag("AllowCommands", flag.Lookup("allow-commands"))
	viper.BindPFlag("AllowEdit", flag.Lookup("allow-edit"))
	viper.BindPFlag("AlowNew", flag.Lookup("allow-new"))
	viper.BindPFlag("AllowPublish", flag.Lookup("allow-publish"))
	viper.BindPFlag("Locale", flag.Lookup("locale"))
	viper.BindPFlag("StaticGen", flag.Lookup("staticgen"))
	viper.BindPFlag("NoAuth", flag.Lookup("no-auth"))
	viper.BindPFlag("BaseURL", flag.Lookup("baseurl"))
	viper.BindPFlag("PrefixURL", flag.Lookup("prefixurl"))

	viper.SetConfigName("filemanager")
	viper.AddConfigPath(".")
}

func printVersion() {
	fmt.Println("filemanager version", filemanager.Version)
	os.Exit(0)
}

func main() {
	setupViper()
	flag.Parse()

	if showVer {
		printVersion()
	}

	// Add a configuration file if set.
	if config != "" {
		ext := filepath.Ext(config)
		dir := filepath.Dir(config)
		config = strings.TrimSuffix(config, ext)

		if dir != "" {
			viper.AddConfigPath(dir)
			config = strings.TrimPrefix(config, dir)
		}

		viper.SetConfigName(config)
	}

	// Read configuration from a file if exists.
	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigParseError); ok {
			panic(err)
		}
	}

	// Set up process log before anything bad happens.
	switch viper.GetString("Logger") {
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stderr)
	case "":
		log.SetOutput(ioutil.Discard)
	default:
		log.SetOutput(&lumberjack.Logger{
			Filename:   logfile,
			MaxSize:    100,
			MaxAge:     14,
			MaxBackups: 10,
		})
	}

	// Builds the address and a listener.
	laddr := viper.GetString("Address") + ":" + viper.GetString("Port")
	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	// Tell the user the port in which is listening.
	fmt.Println("Listening on", listener.Addr().String())

	// Starts the server.
	if err := http.Serve(listener, handler()); err != nil {
		log.Fatal(err)
	}
}

func handler() http.Handler {
	db, err := storm.Open(viper.GetString("Database"))
	if err != nil {
		log.Fatal(err)
	}

	fm := &filemanager.FileManager{
		NoAuth:    viper.GetBool("NoAuth"),
		BaseURL:   viper.GetString("BaseURL"),
		PrefixURL: viper.GetString("PrefixURL"),
		DefaultUser: &filemanager.User{
			AllowCommands: viper.GetBool("AllowCommands"),
			AllowEdit:     viper.GetBool("AllowEdit"),
			AllowNew:      viper.GetBool("AllowNew"),
			AllowPublish:  viper.GetBool("AllowPublish"),
			Commands:      viper.GetStringSlice("Commands"),
			Rules:         []*filemanager.Rule{},
			Locale:        viper.GetString("Locale"),
			CSS:           "",
			Scope:         viper.GetString("Scope"),
			FileSystem:    fileutils.Dir(viper.GetString("Scope")),
		},
		Store: &filemanager.Store{
			Config: bolt.ConfigStore{DB: db},
			Users:  bolt.UsersStore{DB: db},
			Share:  bolt.ShareStore{DB: db},
		},
		NewFS: func(scope string) filemanager.FileSystem {
			return fileutils.Dir(scope)
		},
	}

	err = fm.Setup()
	if err != nil {
		log.Fatal(err)
	}

	switch viper.GetString("StaticGen") {
	case "hugo":
		hugo := &staticgen.Hugo{
			Root:        viper.GetString("Scope"),
			Public:      filepath.Join(viper.GetString("Scope"), "public"),
			Args:        []string{},
			CleanPublic: true,
		}

		if err = fm.Attach(hugo); err != nil {
			log.Fatal(err)
		}
	case "jekyll":
		jekyll := &staticgen.Jekyll{
			Root:        viper.GetString("Scope"),
			Public:      filepath.Join(viper.GetString("Scope"), "_site"),
			Args:        []string{"build"},
			CleanPublic: true,
		}

		if err = fm.Attach(jekyll); err != nil {
			log.Fatal(err)
		}
	}

	return h.Handler(fm)
}
