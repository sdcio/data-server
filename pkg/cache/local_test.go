package cache

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/iptecharch/data-server/pkg/config"
)

func cleanupCacheDir(path string) error {
	time.Sleep(100 * time.Millisecond)
	// clean up cache dir
	return os.RemoveAll(path)
}

func Test_localCache_Create(t *testing.T) {
	type fields struct {
		cacheConfig *config.CacheConfig
	}
	type args struct {
		ctx  context.Context
		name string
		in2  bool
		in3  bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "simple create",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:  context.TODO(),
				name: "cache",
				in2:  false,
				in3:  false,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		err := tt.fields.cacheConfig.ValidateSetDefaults()
		if err != nil {
			t.Errorf("localCache.Create() failed to validate config: %v", err)
		}
		c, err := NewLocalCache(tt.fields.cacheConfig)
		if err != nil {
			t.Errorf("localCache.Create() failed to create cache object: %v", err)
		}
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// clean up cache dir
				err = cleanupCacheDir(tt.fields.cacheConfig.Dir)
				if err != nil {
					t.Errorf("localCache.Create() failed to cleanup cache dir = %v,", err)
				}
			}()
			if err := c.Create(tt.args.ctx, tt.args.name, tt.args.in2, tt.args.in3); (err != nil) != tt.wantErr {
				t.Errorf("localCache.Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_localCache_List(t *testing.T) {
	type fields struct {
		cacheConfig *config.CacheConfig
	}
	type args struct {
		ctx          context.Context
		createCaches []string
		deleteCaches []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "no_cache_instances",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:          context.Background(),
				createCaches: []string{},
			},
			want:    []string{},
			wantErr: false,
		},
		{
			name: "single_cache_instance",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:          context.Background(),
				createCaches: []string{"cache1"},
				deleteCaches: []string{},
			},
			want:    []string{"cache1"},
			wantErr: false,
		},
		{
			name: "multiple_cache_instance",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:          context.Background(),
				createCaches: []string{"cache1", "cache2"},
			},
			want:    []string{"cache1", "cache2"},
			wantErr: false,
		},
		{
			name: "list after deleting",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:          context.Background(),
				createCaches: []string{"cache1", "cache2"},
				deleteCaches: []string{"cache1"},
			},
			want:    []string{"cache2"},
			wantErr: false,
		},
		{
			name: "list after deleting all",
			fields: fields{
				cacheConfig: &config.CacheConfig{},
			},
			args: args{
				ctx:          context.Background(),
				createCaches: []string{"cache1", "cache2", "cache3"},
				deleteCaches: []string{"cache1", "cache2", "cache3"},
			},
			want:    []string{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// clean up cache dir
				err := cleanupCacheDir(tt.fields.cacheConfig.Dir)
				if err != nil {
					t.Errorf("localCache.List() failed to cleanup cache dir = %v,", err)
				}
			}()
			// init cache config
			err := tt.fields.cacheConfig.ValidateSetDefaults()
			if err != nil {
				t.Errorf("localCache.List() failed to validate config: %v", err)
			}
			// create cache object
			c, err := NewLocalCache(tt.fields.cacheConfig)
			if err != nil {
				t.Errorf("localCache.List() failed to create cache object: %v", err)
			}
			//
			ctx, cancel := context.WithTimeout(tt.args.ctx, 30*time.Second)
			defer cancel()
			// create cache instances to be listed
			for _, name := range tt.args.createCaches {
				err = c.Create(ctx, name, false, false)
				if err != nil {
					t.Errorf("localCache.List() failed to create caches list: %v", err)
				}
			}
			// delete caches
			ctx, cancel = context.WithTimeout(tt.args.ctx, 30*time.Second)
			defer cancel()
			// create cache instances to be listed
			for _, name := range tt.args.deleteCaches {
				err = c.Delete(ctx, name)
				if err != nil {
					t.Errorf("localCache.List() failed to delete caches list: %v", err)
				}
			}

			ctx, cancel = context.WithTimeout(tt.args.ctx, 30*time.Second)
			defer cancel()
			got, err := c.List(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("localCache.List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("localCache.List() = %v, want %v", got, tt.want)
			}
		})
	}
}
