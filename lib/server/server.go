// Copyright 2017 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source
// tree.

package server

import (
	"github.com/freetaxii/freetaxii-server/lib/config"
	"github.com/freetaxii/libstix2/datastore"
)

// --------------------------------------------------
// Setup Handler Structs
// --------------------------------------------------

// ServerHandlerType - This type will hold the data elements required to
// process all TAXII media type requests. Since we are using a single handler
// for multiple taxii messages, we need to know the resource type.
type ServerHandlerType struct {
	Type         string // Used in log messages
	ResourcePath string // This is used in the HTML output and to build the URL for the next resource
	HTMLEnabled  bool
	HTMLTemplate string // The full file path (prefix + html template dir + template filename)
	CollectionID string
	RangeMax     int
	DS           datastore.Datastorer
	Resource     interface{}
}

// These methods will copy the elements found in the main configuration file.
// We do this so that we do not send the entire configuration to a handler.
// Also, this enables us to create a generic handler that can fulfill requests
// for all of the TAXII and STIX handlers because we can pre-format the data to
// be in a consistent and correct from.

/*
NewDiscoveryHandler - This method will copy some of the configuration elements
in to the server handler type.
*/
func NewDiscoveryHandler(cfg config.DiscoveryServiceType) (ServerHandlerType, error) {
	var s ServerHandlerType
	s.Type = "Discovery"
	s.ResourcePath = cfg.ResourcePath
	s.HTMLEnabled = cfg.HTML.Enabled
	s.HTMLTemplate = cfg.HTML.TemplatePath + "/" + cfg.HTML.TemplateFiles.Discovery
	return s, nil
}

// NewAPIRootHandler - This method will make a copy of the elements found in
// the main configuration for this API Root Service and copy them here.
func (s *ServerHandlerType) NewAPIRootHandler(c config.APIRootServiceType) {
	s.Type = "API-Root"
	s.ResourcePath = c.ResourcePath
	s.HTMLEnabled = cfg.HTML.Enabled
	s.HTMLTemplate = cfg.HTML.TemplatePath + "/" + cfg.HTML.TemplateFiles.APIRoot
}

// NewCollectionsHandler - This method will make a copy of the elements found in
// the main configuration for this Collections Service and copy them here.
func (s *ServerHandlerType) NewCollectionsHandler(c config.APIRootServiceType) {
	s.Type = "Collections"
	s.ResourcePath = c.Collections.ResourcePath
	s.HTMLEnabled = cfg.HTML.Enabled
	s.HTMLTemplate = cfg.HTML.TemplatePath + "/" + cfg.HTML.TemplateFiles.Collections
}

// NewCollectionHandler - This method will make a copy of the elements found in
// the main configuration for this Collection Service and copy them here.
func (s *ServerHandlerType) NewCollectionHandler(c config.APIRootServiceType, path string) {
	s.Type = "Collection"
	s.ResourcePath = path
	s.HTMLEnabled = cfg.HTML.Enabled
	s.HTMLTemplate = cfg.HTML.TemplatePath + "/" + cfg.HTML.TemplateFiles.Collection
}
