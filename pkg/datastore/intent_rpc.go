// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datastore

import (
	"context"
	"errors"
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/sdcio/data-server/pkg/datastore/target"
)

var rawIntentPrefix = "__raw_intent__"

const (
	intentRawNameSep = "_"
)

var ErrIntentNotFound = errors.New("intent not found")

func (d *Datastore) applyIntent(ctx context.Context, source target.TargetSource) (*sdcpb.SetDataResponse, error) {
	var err error

	var rsp *sdcpb.SetDataResponse
	// send set request only if there are updates and/or deletes

	if d.sbi == nil {
		return nil, fmt.Errorf("%s is not connected", d.config.Name)
	}

	rsp, err = d.sbi.Set(ctx, source)
	if err != nil {
		return nil, err
	}
	log.Debugf("datastore %s SetResponse from SBI: %v", d.config.Name, rsp)

	return rsp, nil
}
