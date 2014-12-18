package main

import (
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/signals"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongostat"
	"os"
	"strconv"
	"time"
)

func main() {
	go signals.Handle()
	// initialize command-line opts
	opts := options.New(
		"mongostat",
		"[options] <polling interval in seconds>",
		options.EnabledOptions{Connection: true, Auth: true, Namespace: false})

	// add mongostat-specific options
	statOpts := &mongostat.StatOptions{}
	opts.AddOptions(statOpts)

	args, err := opts.Parse()
	if err != nil {
		log.Logf(log.Always, "error parsing command line options: %v", err)
		log.Logf(log.Always, "try 'mongostat --help' for more information")
		os.Exit(util.ExitBadOptions)
	}

	log.SetVerbosity(opts.Verbosity)

	sleepInterval := 1
	if len(args) > 0 {
		if len(args) != 1 {
			log.Logf(log.Always, "too many positional arguments: %v", args)
			log.Logf(log.Always, "try 'mongostat --help' for more information")
			os.Exit(util.ExitBadOptions)
		}
		sleepInterval, err = strconv.Atoi(args[0])
		if err != nil {
			log.Logf(log.Always, "invalid sleep interval: %v", args[0])
			os.Exit(util.ExitBadOptions)
		}
		if sleepInterval < 1 {
			log.Logf(log.Always, "sleep interval must be at least 1 second")
			os.Exit(util.ExitBadOptions)
		}
	}

	// print help, if specified
	if opts.PrintHelp(false) {
		return
	}

	// print version, if specified
	if opts.PrintVersion() {
		return
	}

	if opts.Auth.Username != "" && opts.Auth.Source == "" {
		log.Logf(log.Always, "--authenticationDatabase is required")
		os.Exit(util.ExitBadOptions)
	}

	var formatter mongostat.LineFormatter
	formatter = &mongostat.GridLineFormatter{!statOpts.NoHeaders, 10}
	if statOpts.Json {
		formatter = &mongostat.JSONLineFormatter{}
	}

	seedHosts := util.CreateConnectionAddrs(opts.Host, opts.Port)
	var cluster mongostat.ClusterMonitor
	if statOpts.Discover || len(seedHosts) > 1 {
		cluster = &mongostat.AsyncClusterMonitor{
			ReportChan:    make(chan mongostat.StatLine),
			LastStatLines: map[string]*mongostat.StatLine{},
			Formatter:     formatter,
		}
	} else {
		cluster = &mongostat.SyncClusterMonitor{
			ReportChan: make(chan mongostat.StatLine),
			Formatter:  formatter,
		}
	}

	var discoverChan chan string
	if statOpts.Discover {
		discoverChan = make(chan string, 128)
	}

	opts.Direct = true
	_, setName := util.ParseConnectionString(opts.Host)
	opts.ReplicaSetName = setName
	stat := &mongostat.MongoStat{
		Options:       opts,
		StatOptions:   statOpts,
		Nodes:         map[string]*mongostat.NodeMonitor{},
		Discovered:    discoverChan,
		SleepInterval: time.Duration(sleepInterval) * time.Second,
		Cluster:       cluster,
	}

	for _, v := range seedHosts {
		stat.AddNewNode(v)
	}

	// kick it off
	err = stat.Run()
	if err != nil {
		log.Logf(log.Always, "Failed: %v", err)
		os.Exit(util.ExitError)
	}
}
