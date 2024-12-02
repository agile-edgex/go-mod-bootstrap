//
// Copyright (C) 2021 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"github.com/agile-edge/go-mod-bootstrap/v3/di"
	"github.com/agile-edge/go-mod-core-contracts/v3/clients/interfaces"
)

// CommonClientName contains the name of the CommonClient instance in the DIC.
var CommonClientName = di.TypeInstanceToName((*interfaces.CommonClient)(nil))

// CommonClientFrom helper function queries the DIC and returns the CommonClient instance.
func CommonClientFrom(get di.Get) interfaces.CommonClient {
	client, ok := get(CommonClientName).(interfaces.CommonClient)
	if !ok {
		return nil
	}

	return client
}
