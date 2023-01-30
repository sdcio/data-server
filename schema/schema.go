package schema

import (
	"fmt"
	"sync"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/openconfig/goyang/pkg/yang"
	log "github.com/sirupsen/logrus"
)

type Schema struct {
	config *config.SchemaConfig

	m       *sync.RWMutex
	root    *yang.Entry
	modules *yang.Modules
	status  string
}

func NewSchema(sCfg *config.SchemaConfig) (*Schema, error) {
	sc := &Schema{
		config:  sCfg,
		m:       new(sync.RWMutex),
		root:    &yang.Entry{},
		modules: yang.NewModules(),
	}
	now := time.Now()
	var err error
	sCfg.Files, err = findYangFiles(sCfg.Files)
	if err != nil {
		sc.status = "failed"
		return sc, err
	}
	err = sc.readYANGFiles()
	if err != nil {
		sc.status = "failed"
		return sc, err
	}
	sc.root = &yang.Entry{
		Name: "root",
		Kind: yang.DirectoryEntry,
		Dir:  make(map[string]*yang.Entry, len(sc.modules.Modules)),
		Annotation: map[string]interface{}{
			"schemapath": "/",
			"root":       true,
		},
	}

	for _, m := range sc.modules.Modules {
		e := yang.ToEntry(m)
		sc.root.Dir[e.Name] = e
	}
	sc.status = "ok"
	log.Infof("schema %s parsed in %s", sc.UniqueName(), time.Since(now))
	return sc, nil
}

func (s *Schema) Reload() (*Schema, error) {
	s.status = "reloading"
	return NewSchema(s.config)
}

func (s *Schema) UniqueName() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%s@%s@%s", s.config.Name, s.config.Vendor, s.config.Version)
}

func (s *Schema) Name() string {
	if s == nil {
		return ""
	}
	return s.config.Name
}

func (s *Schema) Vendor() string {
	if s == nil {
		return ""
	}
	return s.config.Vendor
}

func (s *Schema) Version() string {
	if s == nil {
		return ""
	}
	return s.config.Version
}

func (s *Schema) Files() []string {
	return s.config.Files
}

func (s *Schema) Dirs() []string {
	return s.config.Directories
}
