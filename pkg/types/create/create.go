// Copyright 2016 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package create

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	_ "github.com/containernetworking/cni/pkg/types/020"
	_ "github.com/containernetworking/cni/pkg/types/040"
	_ "github.com/containernetworking/cni/pkg/types/100"
	convert "github.com/containernetworking/cni/pkg/types/internal"
)

// DecodeVersion returns the CNI version from CNI configuration or result JSON,
// or an error if the operation could not be performed.
func DecodeVersion(jsonBytes []byte) (string, error) {
	var conf struct {
		CNIVersion string `json:"cniVersion"`
	}
	/*解析conf中的cniVersion*/
	err := json.Unmarshal(jsonBytes, &conf)
	if err != nil {
		return "", fmt.Errorf("decoding version from network config: %w", err)
	}
	if conf.CNIVersion == "" {
		/*如未提供，认为0.1.0版本*/
		return "0.1.0", nil
	}
	
	/*否则返回指定版本*/
	return conf.CNIVersion, nil
}

// Create creates a CNI Result using the given JSON with the expected
// version, or an error if the creation could not be performed
func Create(version string, bytes []byte) (types.Result, error) {
	/*按版本返回对应的解析的结果*/
	return convert.Create(version, bytes)
}

// CreateFromBytes creates a CNI Result from the given JSON, automatically
// detecting the CNI spec version of the result. An error is returned if the
// operation could not be performed.
func CreateFromBytes(bytes []byte) (types.Result, error) {
	/*cni插件执行的结果是json串，解码其中指明的cniVersion*/
	version, err := DecodeVersion(bytes)
	if err != nil {
		return nil, err
	}
	/*构造此version对应的返回值解析*/
	return convert.Create(version, bytes)
}
