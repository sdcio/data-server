package config

import (
	"fmt"
	"strings"
)

// SyncMode selects how a gNMI sync collects device state.
type SyncMode string

const (
	SyncModeOnChange SyncMode = "on-change"
	SyncModeSample   SyncMode = "sample"
	SyncModeOnce     SyncMode = "once"
	SyncModeGet      SyncMode = "get"
)

// ParseSyncMode resolves a case-insensitive sync mode name.
func ParseSyncMode(s string) (SyncMode, error) {
	switch m := SyncMode(strings.ToLower(s)); m {
	case SyncModeOnChange, SyncModeSample, SyncModeOnce, SyncModeGet:
		return m, nil
	}
	return "", fmt.Errorf("unknown sync mode %q", s)
}

func (m *SyncMode) UnmarshalText(b []byte) error { return unmarshalEnum(m, b, ParseSyncMode) }

// CommitDatastore names the NETCONF datastore that is committed to.
type CommitDatastore string

const (
	NcCommitDatastoreRunning   CommitDatastore = "running"
	NcCommitDatastoreCandidate CommitDatastore = "candidate"
)

// ParseCommitDatastore resolves a case-insensitive NETCONF datastore name.
func ParseCommitDatastore(s string) (CommitDatastore, error) {
	switch d := CommitDatastore(strings.ToLower(s)); d {
	case NcCommitDatastoreRunning, NcCommitDatastoreCandidate:
		return d, nil
	}
	return "", fmt.Errorf("unknown commit-datastore: %s. Must be one of %s, %s", s, NcCommitDatastoreCandidate, NcCommitDatastoreRunning)
}

func (d *CommitDatastore) UnmarshalText(b []byte) error {
	return unmarshalEnum(d, b, ParseCommitDatastore)
}

// NCVersion is a preferred NETCONF protocol version.
type NCVersion string

const (
	NCVersion10 NCVersion = "1.0"
	NCVersion11 NCVersion = "1.1"
)

// ParseNCVersion resolves a NETCONF version string.
func ParseNCVersion(s string) (NCVersion, error) {
	switch v := NCVersion(s); v {
	case NCVersion10, NCVersion11:
		return v, nil
	}
	return "", fmt.Errorf("unknown preferred-nc-version %q. Must be one of %s, %s", s, NCVersion10, NCVersion11)
}

func (v *NCVersion) UnmarshalText(b []byte) error { return unmarshalEnum(v, b, ParseNCVersion) }

// CacheType selects where the cache lives.
type CacheType string

const (
	CacheTypeLocal  CacheType = "local"
	CacheTypeRemote CacheType = "remote"
)

// ParseCacheType resolves a case-insensitive cache type name.
func ParseCacheType(s string) (CacheType, error) {
	switch t := CacheType(strings.ToLower(s)); t {
	case CacheTypeLocal, CacheTypeRemote:
		return t, nil
	}
	return "", fmt.Errorf("unknown cache type %q", s)
}

func (t *CacheType) UnmarshalText(b []byte) error { return unmarshalEnum(t, b, ParseCacheType) }

// unmarshalEnum decodes text through parse; empty text stays unset so
// defaulting can fill it later.
func unmarshalEnum[T ~string](dst *T, b []byte, parse func(string) (T, error)) error {
	if len(b) == 0 {
		*dst = ""
		return nil
	}
	v, err := parse(string(b))
	if err != nil {
		return err
	}
	*dst = v
	return nil
}
