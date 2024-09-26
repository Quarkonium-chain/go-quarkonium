// Copyright (C) 2019-2024 Algorand, Inc.
// This file is part of go-algorand
//
// go-algorand is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// go-algorand is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with go-algorand.  If not, see <https://www.gnu.org/licenses/>.

package trackerdb

import (
	"github.com/Quarkonium-chain/go-quarkonium/crypto"
	"github.com/Quarkonium-chain/go-quarkonium/data/basics"
	"github.com/Quarkonium-chain/go-quarkonium/protocol"
	"github.com/Quarkonium-chain/go-quarkonium/util/db"
)

// Params contains parameters for initializing trackerDB
type Params struct {
	InitAccounts      map[basics.Address]basics.AccountData
	InitProto         protocol.ConsensusVersion
	GenesisHash       crypto.Digest
	FromCatchpoint    bool
	CatchpointEnabled bool
	DbPathPrefix      string
	BlockDb           db.Pair
}

// InitParams params used during db init
type InitParams struct {
	SchemaVersion   int32
	VacuumOnStartup bool
}
