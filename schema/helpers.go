package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/openconfig/goyang/pkg/yang"
)

func (sc *Schema) readYANGFiles() error {
	if len(sc.config.Files) == 0 {
		return nil
	}

	for _, dirpath := range sc.config.Directories {
		expanded, err := yang.PathsWithModules(dirpath)
		if err != nil {
			return err
		}

		sc.modules.AddPath(expanded...)
	}
	excludeRegexes := make([]*regexp.Regexp, 0, len(sc.config.Excludes))
	for _, e := range sc.config.Excludes {
		r, err := regexp.Compile(e)
		if err != nil {
			return err
		}
		excludeRegexes = append(excludeRegexes, r)
	}

MAIN:
	for _, name := range sc.config.Files {
		for _, r := range excludeRegexes {
			if r.MatchString(name) {
				continue MAIN
			}
		}
		err := sc.modules.Read(name)
		if err != nil {
			return err
		}
	}

	if errors := sc.modules.Process(); len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "yang processing error: %v\n", e)
		}
		return fmt.Errorf("yang processing failed with %d errors", len(errors))
	}
	return nil
}

func resolveGlobs(globs []string) ([]string, error) {
	results := make([]string, 0, len(globs))
	for _, pattern := range globs {
		for _, p := range strings.Split(pattern, ",") {
			if strings.ContainsAny(p, `*?[`) {
				// is a glob pattern
				matches, err := filepath.Glob(p)
				if err != nil {
					return nil, err
				}
				results = append(results, matches...)
			} else {
				// is not a glob pattern ( file or dir )
				results = append(results, p)
			}
		}
	}
	return ExpandOSPaths(results)
}

func walkDir(path, ext string) ([]string, error) {
	fs := make([]string, 0)
	err := filepath.Walk(path,
		func(path string, _ os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fi, err := os.Stat(path)
			if err != nil {
				return err
			}
			switch mode := fi.Mode(); {
			case mode.IsRegular():
				if filepath.Ext(path) == ext {
					fs = append(fs, path)
				}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return fs, nil
}

func findYangFiles(files []string) ([]string, error) {
	yfiles := make([]string, 0, len(files))
	for _, file := range files {
		fi, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		switch mode := fi.Mode(); {
		case mode.IsDir():
			fls, err := walkDir(file, ".yang")
			if err != nil {
				return nil, err
			}
			yfiles = append(yfiles, fls...)
		case mode.IsRegular():
			if filepath.Ext(file) == ".yang" {
				yfiles = append(yfiles, file)
			}
		}
	}
	return yfiles, nil
}

func ExpandOSPaths(paths []string) ([]string, error) {
	var err error
	for i := range paths {
		paths[i], err = expandOSPath(paths[i])
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func expandOSPath(p string) (string, error) {
	if p == "-" || p == "" {
		return p, nil
	}
	if strings.HasPrefix(p, "http://") ||
		strings.HasPrefix(p, "https://") ||
		strings.HasPrefix(p, "sftp://") ||
		strings.HasPrefix(p, "ftp://") {
		return p, nil
	}
	np, err := homedir.Expand(p)
	if err != nil {
		return "", fmt.Errorf("path %q: %v", p, err)
	}
	if !filepath.IsAbs(np) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("path %q: %v", p, err)
		}
		np = filepath.Join(cwd, np)
	}
	_, err = os.Stat(np)
	if err != nil {
		return "", err
	}
	return np, nil
}
