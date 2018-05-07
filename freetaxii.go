// Copyright 2017 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source
// tree.

package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/freetaxii/freetaxii-server/lib/config"
	"github.com/freetaxii/freetaxii-server/lib/server"
	"github.com/freetaxii/libstix2/datastore"
	"github.com/freetaxii/libstix2/datastore/sqlite3"
	"github.com/freetaxii/libstix2/resources"
	"github.com/gologme/log"
	"github.com/gorilla/mux"
	"github.com/pborman/getopt"
)

/*
These global variables hold build information. The Build variable will be
populated by the Makefile and uses the Git Head hash as its identifier.
These variables are used in the console output for --version and --help.
*/
var (
	Version = "0.2.1"
	Build   string
)

func main() {
	configFileName := processCommandLineFlags()

	// Keep track of the number of services that are started
	serviceCounter := 0

	// --------------------------------------------------
	// Setup logger
	// --------------------------------------------------
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.EnableLevel("info")
	logger.EnableLevel("debug")

	// --------------------------------------------------
	// Load System and Server Configuration
	// --------------------------------------------------
	config, configError := config.New(logger, configFileName)
	if configError != nil {
		logger.Fatalln(configError)
	}
	logger.Traceln("TRACE: System Configuration Dump")
	logger.Tracef("%+v\n", config)

	// --------------------------------------------------
	// Setup Logging File
	// --------------------------------------------------
	// TODO
	// Need to make the directory if it does not already exist
	// To do this, we need to split the filename from the directory, we will want to only
	// take the last bit in case there is multiple directories /etc/foo/bar/stuff.log

	// Only enable logging to a file if it is turned on in the configuration file
	if config.Logging.Enabled == true {
		logFile, err := os.OpenFile(config.Logging.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			logger.Fatalf("ERROR: can not open file: %v", err)
		}
		defer logFile.Close()
		logger.SetOutput(logFile)
	}

	// --------------------------------------------------
	// Setup Database Connection
	// --------------------------------------------------
	var ds datastore.Datastorer
	switch config.Global.DbType {
	case "sqlite3":
		databaseFilename := config.Global.Prefix + config.Global.DbFile
		ds = sqlite3.New(databaseFilename)
	default:
		logger.Fatalln("ERROR: unknown database type, or no database type defined in the server global configuration")
	}
	defer ds.Close()

	// --------------------------------------------------
	//
	// Configure HTTP Router
	//
	// --------------------------------------------------

	router := mux.NewRouter()
	config.Router = router

	// --------------------------------------------------
	//
	// Start Server
	//
	// --------------------------------------------------

	logger.Println("Starting FreeTAXII Server")

	// --------------------------------------------------
	//
	// Start a Discovery Service handler
	//
	// --------------------------------------------------
	// This will look to see if there are any Discovery services defined in the
	// configuration file. If there are, loop through the list and setup handlers
	// for each one of them. The HandleFunc takes in a copy of the Discovery
	// Resource and the extra meta data that it needs to process the request.

	if config.DiscoveryServer.Enabled == true {
		for _, c := range config.DiscoveryServer.Services {
			if c.Enabled == true {

				// Configuration for this specific instance and its resource
				ts, _ := server.NewDiscoveryHandler(logger, c, config.DiscoveryResources[c.ResourceID])

				logger.Infoln("Starting TAXII Discovery service at:", c.FullPath)
				router.HandleFunc(c.FullPath, ts.DiscoveryHandler).Methods("GET")
				serviceCounter++
			}
		}
	}

	// --------------------------------------------------
	// Start an API Root Service handler
	// Example: /api1/
	// --------------------------------------------------
	// This will look to see if there are any API Root services defined
	// in the config file. If there are, it will loop through the list
	// and setup handlers for each one of them. The HandleFunc passes in
	// copy of the API Root Resource and the extra meta data that it
	// needs to process the request.

	if config.APIRootServer.Enabled == true {
		for _, api := range config.APIRootServer.Services {
			if api.Enabled == true {

				logger.Infoln("Starting TAXII API Root service at:", api.FullPath)
				ts, _ := server.NewAPIRootHandler(logger, api, config.APIRootResources[api.ResourceID])
				router.HandleFunc(api.FullPath, ts.APIRootHandler).Methods("GET")
				serviceCounter++

				// --------------------------------------------------
				// Start a Collections Service handler
				// Example: /api1/collections/
				// --------------------------------------------------
				// This will look to see if the Collections service is enabled
				// in the configuration file for a given API Root.

				if api.Collections.Enabled == true {

					collectionsSrv, _ := server.NewCollectionsHandler(logger, api)
					collections := resources.NewCollections()

					// We need to look in to this instance of the API Root and find out which collections are tied to it
					// Then we can use that ID to pull from the collections list and add them to this list of valid collections
					for _, c := range api.Collections.ResourceIDs {

						// If enabled, only add the collection to the list if the collection can either be read or written to
						if config.CollectionResources[c].CanRead == true || config.CollectionResources[c].CanWrite == true {
							col := config.CollectionResources[c]
							collections.AddCollection(&col)
						}
					}
					collectionsSrv.Resource = collections

					logger.Infoln("Starting TAXII Collections service of:", api.Collections.FullPath)
					router.HandleFunc(collectionsSrv.URLPath, collectionsSrv.CollectionsHandler).Methods("GET")

					// --------------------------------------------------
					// Start a Collection handler
					// Example: /api1/collections/9cfa669c-ee94-4ece-afd2-f8edac37d8fd/
					// --------------------------------------------------
					// This will look to see which collections are defined for this
					// Collections group in this API Root. If they are enabled, it
					// will setup handlers for it.
					// The HandleFunc passes in copy of the Collection Resource and the extra meta data
					// that it needs to process the request.

					for _, c := range api.Collections.ResourceIDs {

						resourceCollectionIDPath := collectionsSrv.URLPath + config.CollectionResources[c].ID + "/"

						collectionSrv, _ := server.NewCollectionHandler(logger, api, resourceCollectionIDPath)
						collectionSrv.Resource = config.CollectionResources[c]

						logger.Infoln("Starting TAXII Collection service of:", resourceCollectionIDPath)

						// We do not need to check to see if the collection is enabled
						// and readable/writable because that was already done
						// TODO add support for post if the collection is writable
						router.HandleFunc(collectionSrv.URLPath, collectionSrv.CollectionHandler).Methods("GET")

						// --------------------------------------------------
						// Start an Objects handler
						// Example: /api1/collections/9cfa669c-ee94-4ece-afd2-f8edac37d8fd/objects/
						// --------------------------------------------------
						var objectsSrv server.ServerHandlerType
						objectsSrv.URLPath = resourceCollectionIDPath + "objects/"
						objectsSrv.HTMLEnabled = api.HTML.Enabled.Value
						objectsSrv.HTMLTemplate = api.HTML.FullTemplatePath + api.HTML.TemplateFiles.Objects.Value
						objectsSrv.CollectionID = config.CollectionResources[c].ID
						objectsSrv.DS = ds

						// --------------------------------------------------
						// Start a Objects and Object by ID handlers
						// --------------------------------------------------

						logger.Infoln("Starting TAXII Object service of:", objectsSrv.URLPath)
						config.Router.HandleFunc(objectsSrv.URLPath, objectsSrv.ObjectsServerHandler).Methods("GET")

						logger.Infoln("Starting TAXII Object service of:", objectsSrv.URLPath)
						objectsSrv.URLPath = resourceCollectionIDPath + "objects/" + "{objectid}/"
						config.Router.HandleFunc(objectsSrv.URLPath, objectsSrv.ObjectsByIDServerHandler).Methods("GET")

						// --------------------------------------------------
						// Start a Manigest handler
						// Example: /api1/collections/9cfa669c-ee94-4ece-afd2-f8edac37d8fd/manifest/
						// --------------------------------------------------
						var manifestSrv server.ServerHandlerType
						manifestSrv.URLPath = resourceCollectionIDPath + "manifest/"
						manifestSrv.HTMLEnabled = api.HTML.Enabled.Value
						manifestSrv.HTMLTemplate = api.HTML.FullTemplatePath + api.HTML.TemplateFiles.Manifest.Value
						manifestSrv.CollectionID = config.CollectionResources[c].ID
						manifestSrv.DS = ds

						// --------------------------------------------------
						// Start a Manifest handlers
						// --------------------------------------------------
						logger.Infoln("Starting TAXII Manifest service of:", manifestSrv.URLPath)
						config.Router.HandleFunc(manifestSrv.URLPath, manifestSrv.ManifestServerHandler).Methods("GET")

					} // End for loop api.Collections.ResourceIDs
				} // End if Collections.Enabled == true
			} // End if api.Enabled == true
		} // End for loop API Root Services
	} // End if APIRootServer.Enabled == true

	// --------------------------------------------------
	//
	// Fail if no services are running
	//
	// --------------------------------------------------

	if serviceCounter == 0 {
		logger.Fatalln("No TAXII services defined")
	}

	// --------------------------------------------------
	//
	// Listen for Incoming Connections
	//
	// --------------------------------------------------

	if config.Global.Protocol == "http" {
		logger.Infoln("Listening on:", config.Global.Listen)
		logger.Fatalln(http.ListenAndServe(config.Global.Listen, router))
	} else if config.Global.Protocol == "https" {
		// --------------------------------------------------
		// Configure TLS settings
		// --------------------------------------------------
		// TODO move TLS elements to configuration file
		tlsConfig := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}
		tlsServer := &http.Server{
			Addr:         config.Global.Listen,
			Handler:      router,
			TLSConfig:    tlsConfig,
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		}

		tlsKeyPath := "etc/tls/" + config.Global.TLSKey
		tlsCrtPath := "etc/tls/" + config.Global.TLSCrt
		logger.Fatalln(tlsServer.ListenAndServeTLS(tlsCrtPath, tlsKeyPath))
	} else {
		logger.Fatalln("No valid protocol was defined in the configuration file")
	} // end if statement
}

// --------------------------------------------------
//
// Private functions
//
// --------------------------------------------------

func startAPIRootServer() {

}

/*
processCommandLineFlags - This function will process the command line flags
and will print the version or help information as needed.
*/
func processCommandLineFlags() string {
	defaultServerConfigFilename := "etc/freetaxii.conf"
	sOptServerConfigFilename := getopt.StringLong("config", 'c', defaultServerConfigFilename, "System Configuration File", "string")
	bOptHelp := getopt.BoolLong("help", 0, "Help")
	bOptVer := getopt.BoolLong("version", 0, "Version")

	getopt.HelpColumn = 35
	getopt.DisplayWidth = 120
	getopt.SetParameters("")
	getopt.Parse()

	// Lets check to see if the version command line flag was given. If it is
	// lets print out the version infomration and exit.
	if *bOptVer {
		printOutputHeader()
		os.Exit(0)
	}

	// Lets check to see if the help command line flag was given. If it is lets
	// print out the help information and exit.
	if *bOptHelp {
		printOutputHeader()
		getopt.Usage()
		os.Exit(0)
	}
	return *sOptServerConfigFilename
}

/*
printOutputHeader - This function will print a header for all console output
*/
func printOutputHeader() {
	fmt.Println("")
	fmt.Println("FreeTAXII Server")
	fmt.Println("Copyright: Bret Jordan")
	fmt.Println("Version:", Version)
	if Build != "" {
		fmt.Println("Build:", Build)
	}
	fmt.Println("")
}
