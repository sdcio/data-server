package api

import (
	"reflect"
	"testing"
)

func fullSpec() DatastoreSpec {
	tgt := func() Target {
		return Target{
			Type:        TargetTypeNetconf,
			Address:     "10.0.0.1",
			Port:        830,
			Credentials: &Credentials{Username: "u", Password: "p", Token: "t"},
			TLS:         &TLS{CA: "ca", Cert: "cert", Key: "key", SkipVerify: true},
			ProtocolOptions: ProtocolOptions{
				GNMI:    &GNMIOptions{Encoding: "JSON_IETF", TargetName: "tn"},
				Netconf: &NetconfOptions{IncludeNs: true, OperationWithNs: true, UseOperationRemove: true, CommitCandidate: CommitCandidateRunning},
			},
		}
	}
	syncTgt := tgt()
	return DatastoreSpec{
		Name:   "ds1",
		Target: tgt(),
		Schema: SchemaRef{Vendor: "nokia", Version: "24.3"},
		Sync: Sync{
			Validate: true, Buffer: 5, WriteWorkers: 3,
			Configs: []SyncConfig{{Name: "c", Target: &syncTgt, Paths: []string{"/a", "/b"}, Mode: SyncModeSample, Interval: 10}},
		},
	}
}

func TestHashStable(t *testing.T) {
	a, b := ComputeCreateDSHash(fullSpec()), ComputeCreateDSHash(fullSpec())
	if a != b {
		t.Fatalf("hash differs for equal specs: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("unexpected digest length %d", len(a))
	}
}

// Golden value pins the canonical form: changing it forces every datastore to be recreated.
func TestHashGolden(t *testing.T) {
	const want = "0532c42716855523d20c478edb017f08cf480320a7f7c3c060db8d2f8200f082"
	if got := ComputeCreateDSHash(fullSpec()); got != want {
		t.Fatalf("hash = %s, want %s", got, want)
	}
}

// Every leaf field of the spec must influence the hash, so a change recreates the datastore.
func TestHashChangesOnEveryField(t *testing.T) {
	base := ComputeCreateDSHash(fullSpec())
	s := fullSpec()
	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			walk(path, v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		case reflect.Slice:
			if v.Len() > 0 {
				walk(path+"[0]", v.Index(0))
			}
		default:
			orig := reflect.New(v.Type()).Elem()
			orig.Set(v)
			switch v.Kind() {
			case reflect.String:
				v.SetString(v.String() + "x")
			case reflect.Bool:
				v.SetBool(!v.Bool())
			case reflect.Int, reflect.Int32, reflect.Int64:
				v.SetInt(v.Int() + 1)
			case reflect.Uint32, reflect.Uint64:
				v.SetUint(v.Uint() + 1)
			default:
				t.Fatalf("%s: unhandled kind %s", path, v.Kind())
			}
			if ComputeCreateDSHash(s) == base {
				t.Errorf("hash ignores %s", path)
			}
			v.Set(orig)
		}
	}
	walk("Spec", reflect.ValueOf(&s).Elem())
}

func TestHashNilVersusEmpty(t *testing.T) {
	a := fullSpec()
	b := fullSpec()
	a.Target.Credentials = nil
	b.Target.Credentials = &Credentials{}
	if ComputeCreateDSHash(a) == ComputeCreateDSHash(b) {
		t.Fatal("nil and empty credentials hash equal")
	}
}

func TestHashExtra(t *testing.T) {
	s := fullSpec()
	base := ComputeCreateDSHash(s)
	withExtra := ComputeCreateDSHash(s, "1")
	if withExtra == base {
		t.Fatal("extra ignored")
	}
	if withExtra != ComputeCreateDSHash(s, "1") {
		t.Fatal("extra unstable")
	}
	if ComputeCreateDSHash(s, "a", "b") == ComputeCreateDSHash(s, "ab") {
		t.Fatal("extra boundaries collide")
	}
}
