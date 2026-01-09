package gnmi

type GnmiSync interface {
	Start() error
	Stop() error
	Name() string
}
