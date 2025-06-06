package types

type LeafEntry interface {
	MarkDelete(onlyIntended bool)
	MarkNew()
}

type UpdateInsertFlags struct {
	new          bool
	delete       bool
	onlyIntended bool
}

// NewUpdateInsertFlags returns a new *UpdateInsertFlags instance
// with all values set to false, so not new, and not marked for deletion
func NewUpdateInsertFlags() *UpdateInsertFlags {
	return &UpdateInsertFlags{}
}

func (f *UpdateInsertFlags) SetDeleteFlag() *UpdateInsertFlags {
	f.delete = true
	f.new = false
	return f
}

func (f *UpdateInsertFlags) SetDeleteOnlyUpdatedFlag() *UpdateInsertFlags {
	f.delete = true
	f.onlyIntended = true
	f.new = false
	return f
}

func (f *UpdateInsertFlags) SetNewFlag() *UpdateInsertFlags {
	f.new = true
	f.delete = false
	f.onlyIntended = false
	return f
}

func (f *UpdateInsertFlags) GetDeleteFlag() bool {
	return f.delete
}

func (f *UpdateInsertFlags) GetDeleteOnlyIntendedFlag() bool {
	return f.onlyIntended
}

func (f *UpdateInsertFlags) GetNewFlag() bool {
	return f.new
}

func (f *UpdateInsertFlags) Apply(le LeafEntry) *UpdateInsertFlags {
	if f.delete {
		le.MarkDelete(f.onlyIntended)
		return f
	}
	if f.new {
		le.MarkNew()
		return f
	}
	return f
}
