package api

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
)

// ComputeCreateDSHash returns a stable hex digest of the spec. Equal specs hash
// equal and any field change alters the digest, so callers recreate a datastore
// when it differs. extra values (for example resource versions of referenced
// objects) are mixed in, in order.
//
// The hash is over the resolved DatastoreSpec, not a caller's own API type, so
// it changes exactly when the datastore the server would build changes. Callers
// that resolve references (secrets, TLS material) pass their versions as extra.
// It lives next to DatastoreSpec so a new field cannot be added without
// hashing it; TestHashChangesOnEveryField enforces this. Changing the encoding
// changes every digest and recreates all datastores.
func ComputeCreateDSHash(spec DatastoreSpec, extra ...string) string {
	w := hashWriter{h: sha256.New()}
	w.str(spec.Name)
	w.target(&spec.Target)
	w.str(spec.Schema.Name)
	w.str(spec.Schema.Vendor)
	w.str(spec.Schema.Version)
	w.i64(int64(len(spec.Sync.Configs)))
	for i := range spec.Sync.Configs {
		c := &spec.Sync.Configs[i]
		w.str(c.Name)
		if w.present(c.Target != nil) {
			w.target(c.Target)
		}
		w.i64(int64(len(c.Paths)))
		for _, p := range c.Paths {
			w.str(p)
		}
		w.i64(int64(c.Mode))
		w.i64(int64(c.Interval))
	}
	w.i64(int64(len(extra)))
	for _, e := range extra {
		w.str(e)
	}
	return hex.EncodeToString(w.h.Sum(nil))
}

// hashWriter writes length-prefixed fields so adjacent values cannot collide.
type hashWriter struct {
	h hash.Hash
}

func (w hashWriter) i64(v int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	w.h.Write(b[:])
}

func (w hashWriter) str(s string) {
	w.i64(int64(len(s)))
	w.h.Write([]byte(s))
}

func (w hashWriter) bool(v bool) {
	if v {
		w.h.Write([]byte{1})
		return
	}
	w.h.Write([]byte{0})
}

// present writes a presence marker and returns ok, so nil and zero values hash apart.
func (w hashWriter) present(ok bool) bool {
	w.bool(ok)
	return ok
}

func (w hashWriter) target(t *Target) {
	w.str(string(t.Type))
	w.str(t.Address)
	w.i64(int64(t.Port))
	if w.present(t.Credentials != nil) {
		w.str(t.Credentials.Username)
		w.str(t.Credentials.Password)
		w.str(t.Credentials.Token)
	}
	if w.present(t.TLS != nil) {
		w.str(t.TLS.CA)
		w.str(t.TLS.Cert)
		w.str(t.TLS.Key)
		w.bool(t.TLS.SkipVerify)
	}
	if g := t.ProtocolOptions.GNMI; w.present(g != nil) {
		w.str(string(g.Encoding))
		w.str(g.TargetName)
	}
	if n := t.ProtocolOptions.Netconf; w.present(n != nil) {
		w.bool(n.IncludeNs)
		w.bool(n.OperationWithNs)
		w.bool(n.UseOperationRemove)
		w.i64(int64(n.CommitCandidate))
	}
}
