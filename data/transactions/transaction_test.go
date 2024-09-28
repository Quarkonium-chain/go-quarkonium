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

package transactions

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Quarkonium-chain/go-quarkonium/config"
	"github.com/Quarkonium-chain/go-quarkonium/crypto"
	"github.com/Quarkonium-chain/go-quarkonium/crypto/merklesignature"
	"github.com/Quarkonium-chain/go-quarkonium/crypto/stateproof"
	"github.com/Quarkonium-chain/go-quarkonium/data/basics"
	"github.com/Quarkonium-chain/go-quarkonium/data/stateproofmsg"
	"github.com/Quarkonium-chain/go-quarkonium/protocol"
	"github.com/Quarkonium-chain/go-quarkonium/test/partitiontest"
)

func TestTransaction_EstimateEncodedSize(t *testing.T) {
	partitiontest.PartitionTest(t)

	addr, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)

	buf := make([]byte, 10)
	crypto.RandBytes(buf[:])

	proto := config.Consensus[protocol.ConsensusCurrentVersion]
	tx := Transaction{
		Type: protocol.PaymentTx,
		Header: Header{
			Sender:     addr,
			Fee:        basics.MicroAlgos{Raw: 100},
			FirstValid: basics.Round(1000),
			LastValid:  basics.Round(1000 + proto.MaxTxnLife),
			Note:       buf,
		},
		PaymentTxnFields: PaymentTxnFields{
			Receiver: addr,
			Amount:   basics.MicroAlgos{Raw: 100},
		},
	}

	require.Equal(t, 200, tx.EstimateEncodedSize())
}

func generateDummyGoNonparticpatingTransaction(addr basics.Address) (tx Transaction) {
	buf := make([]byte, 10)
	crypto.RandBytes(buf[:])

	proto := config.Consensus[protocol.ConsensusCurrentVersion]
	tx = Transaction{
		Type: protocol.KeyRegistrationTx,
		Header: Header{
			Sender:     addr,
			Fee:        basics.MicroAlgos{Raw: proto.MinTxnFee},
			FirstValid: 1,
			LastValid:  300,
		},
		KeyregTxnFields: KeyregTxnFields{
			Nonparticipation: true,
			VoteFirst:        0,
			VoteLast:         0,
		},
	}

	tx.KeyregTxnFields.Nonparticipation = true
	return tx
}

func TestGoOnlineGoNonparticipatingContradiction(t *testing.T) {
	partitiontest.PartitionTest(t)

	// addr has no significance here other than being a normal valid address
	addr, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)

	tx := generateDummyGoNonparticpatingTransaction(addr)
	// Generate keys, they don't need to be good or secure, just present
	v := crypto.GenerateOneTimeSignatureSecrets(1, 1)
	// Also generate a new VRF key
	vrf := crypto.GenerateVRFSecrets()
	tx.KeyregTxnFields = KeyregTxnFields{
		VotePK:           v.OneTimeSignatureVerifier,
		SelectionPK:      vrf.PK,
		VoteKeyDilution:  1,
		VoteFirst:        1,
		VoteLast:         100,
		Nonparticipation: true,
	}
	// this tx tries to both register keys to go online, and mark an account as non-participating.
	// it is not well-formed.
	err = tx.WellFormed(SpecialAddresses{}, config.Consensus[protocol.ConsensusCurrentVersion])
	require.ErrorContains(t, err, "tries to register keys to go online, but nonparticipatory flag is set")
}

func TestGoNonparticipatingWellFormed(t *testing.T) {
	partitiontest.PartitionTest(t)

	// addr has no significance here other than being a normal valid address
	addr, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)

	tx := generateDummyGoNonparticpatingTransaction(addr)
	curProto := config.Consensus[protocol.ConsensusCurrentVersion]

	if !curProto.SupportBecomeNonParticipatingTransactions {
		t.Skipf("Skipping rest of test because current protocol version %v does not support become-nonparticipating transactions", protocol.ConsensusCurrentVersion)
	}

	// this tx is well-formed
	err = tx.WellFormed(SpecialAddresses{}, curProto)
	require.NoError(t, err)
	// but it should stop being well-formed if the protocol does not support it
	curProto.SupportBecomeNonParticipatingTransactions = false
	err = tx.WellFormed(SpecialAddresses{}, curProto)
	require.ErrorContains(t, err, "mark an account as nonparticipating, but")
}

func TestAppCallCreateWellFormed(t *testing.T) {
	partitiontest.PartitionTest(t)

	curProto := config.Consensus[protocol.ConsensusCurrentVersion]
	futureProto := config.Consensus[protocol.ConsensusFuture]
	addr1, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)
	v5 := []byte{0x05}
	v6 := []byte{0x06}

	usecases := []struct {
		tx            Transaction
		proto         config.ConsensusParams
		expectedError string
	}{
		{
			tx: Transaction{
				Type: protocol.ApplicationCallTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  105,
					FirstValid: 100,
				},
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
				},
			},
			proto: curProto,
		},
		{
			tx: Transaction{
				Type: protocol.ApplicationCallTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  105,
					FirstValid: 100,
				},
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 0,
				},
			},
			proto: curProto,
		},
		{
			tx: Transaction{
				Type: protocol.ApplicationCallTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  105,
					FirstValid: 100,
				},
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 3,
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type: protocol.ApplicationCallTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  105,
					FirstValid: 100,
				},
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 0,
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type: protocol.ApplicationCallTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  105,
					FirstValid: 100,
				},
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApprovalProgram:   v5,
					ClearStateProgram: v6,
				},
			},
			proto:         futureProto,
			expectedError: "mismatch",
		},
	}
	for i, usecase := range usecases {
		t.Run(fmt.Sprintf("i=%d", i), func(t *testing.T) {
			err := usecase.tx.WellFormed(SpecialAddresses{}, usecase.proto)
			if usecase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), usecase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWellFormedErrors(t *testing.T) {
	partitiontest.PartitionTest(t)

	curProto := config.Consensus[protocol.ConsensusCurrentVersion]
	futureProto := config.Consensus[protocol.ConsensusFuture]
	protoV27 := config.Consensus[protocol.ConsensusV27]
	protoV28 := config.Consensus[protocol.ConsensusV28]
	protoV32 := config.Consensus[protocol.ConsensusV32]
	protoV36 := config.Consensus[protocol.ConsensusV36]
	addr1, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)
	v5 := []byte{0x05}
	okHeader := Header{
		Sender:     addr1,
		Fee:        basics.MicroAlgos{Raw: 1000},
		LastValid:  105,
		FirstValid: 100,
	}
	usecases := []struct {
		tx            Transaction
		proto         config.ConsensusParams
		expectedError error
	}{
		{
			tx: Transaction{
				Type: protocol.PaymentTx,
				Header: Header{
					Sender: addr1,
					Fee:    basics.MicroAlgos{Raw: 100},
				},
			},
			proto:         protoV27,
			expectedError: makeMinFeeErrorf("transaction had fee %d, which is less than the minimum %d", 100, curProto.MinTxnFee),
		},
		{
			tx: Transaction{
				Type: protocol.PaymentTx,
				Header: Header{
					Sender: addr1,
					Fee:    basics.MicroAlgos{Raw: 100},
				},
			},
			proto: curProto,
		},
		{
			tx: Transaction{
				Type: protocol.PaymentTx,
				Header: Header{
					Sender:     addr1,
					Fee:        basics.MicroAlgos{Raw: 1000},
					LastValid:  100,
					FirstValid: 105,
				},
			},
			proto:         curProto,
			expectedError: fmt.Errorf("transaction invalid range (%d--%d)", 105, 100),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0, // creation
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 1,
				},
			},
			proto:         protoV27,
			expectedError: fmt.Errorf("tx.ExtraProgramPages exceeds MaxExtraAppProgramPages = %d", protoV27.MaxExtraAppProgramPages),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0, // creation
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte("Xjunk"),
				},
			},
			proto:         protoV27,
			expectedError: fmt.Errorf("approval program too long. max len 1024 bytes"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0, // creation
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte("Xjunk"),
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0, // creation
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte(strings.Repeat("X", 1025)),
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("app programs too long. max total len 2048 bytes"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0, // creation
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte(strings.Repeat("X", 1025)),
					ExtraProgramPages: 1,
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 1,
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx.ExtraProgramPages is immutable"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     0,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 4,
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx.ExtraProgramPages exceeds MaxExtraAppProgramPages = %d", futureProto.MaxExtraAppProgramPages),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					ForeignApps:   []basics.AppIndex{10, 11},
				},
			},
			proto: protoV27,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					ForeignApps:   []basics.AppIndex{10, 11, 12},
				},
			},
			proto:         protoV27,
			expectedError: fmt.Errorf("tx.ForeignApps too long, max number of foreign apps is 2"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					ForeignApps:   []basics.AppIndex{10, 11, 12, 13, 14, 15, 16, 17},
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					ForeignAssets: []basics.AssetIndex{14, 15, 16, 17, 18, 19, 20, 21, 22},
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx.ForeignAssets too long, max number of foreign assets is 8"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Accounts:      []basics.Address{{}, {}, {}},
					ForeignApps:   []basics.AppIndex{14, 15, 16, 17},
					ForeignAssets: []basics.AssetIndex{14, 15, 16, 17},
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx references exceed MaxAppTotalTxnReferences = 8"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     1,
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte(strings.Repeat("X", 1025)),
					ExtraProgramPages: 0,
					OnCompletion:      UpdateApplicationOC,
				},
			},
			proto:         protoV28,
			expectedError: fmt.Errorf("app programs too long. max total len %d bytes", curProto.MaxAppProgramLen),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     1,
					ApprovalProgram:   []byte(strings.Repeat("X", 1025)),
					ClearStateProgram: []byte(strings.Repeat("X", 1025)),
					ExtraProgramPages: 0,
					OnCompletion:      UpdateApplicationOC,
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID:     1,
					ApprovalProgram:   v5,
					ClearStateProgram: v5,
					ApplicationArgs: [][]byte{
						[]byte("write"),
					},
					ExtraProgramPages: 1,
					OnCompletion:      UpdateApplicationOC,
				},
			},
			proto:         protoV28,
			expectedError: fmt.Errorf("tx.ExtraProgramPages is immutable"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Boxes:         []BoxRef{{Index: 1, Name: []byte("junk")}},
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx.Boxes[0].Index is 1. Exceeds len(tx.ForeignApps)"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Boxes:         []BoxRef{{Index: 1, Name: []byte("junk")}},
					ForeignApps:   []basics.AppIndex{1},
				},
			},
			proto: futureProto,
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Boxes:         []BoxRef{{Index: 1, Name: []byte("junk")}},
					ForeignApps:   []basics.AppIndex{1},
				},
			},
			proto:         protoV32,
			expectedError: fmt.Errorf("tx.Boxes too long, max number of box references is 0"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Boxes:         []BoxRef{{Index: 1, Name: make([]byte, 65)}},
					ForeignApps:   []basics.AppIndex{1},
				},
			},
			proto:         futureProto,
			expectedError: fmt.Errorf("tx.Boxes[0].Name too long, max len 64 bytes"),
		},
		{
			tx: Transaction{
				Type:   protocol.ApplicationCallTx,
				Header: okHeader,
				ApplicationCallTxnFields: ApplicationCallTxnFields{
					ApplicationID: 1,
					Boxes:         []BoxRef{{Index: 1, Name: make([]byte, 65)}},
					ForeignApps:   []basics.AppIndex{1},
				},
			},
			proto:         protoV36,
			expectedError: nil,
		},
	}
	for _, usecase := range usecases {
		err := usecase.tx.WellFormed(SpecialAddresses{}, usecase.proto)
		require.Equal(t, usecase.expectedError, err)
	}
}

// TestTransactionHash checks that Transaction.ID() is equivalent to the old simpler crypto.HashObj() implementation.
func TestTransactionHash(t *testing.T) {
	partitiontest.PartitionTest(t)

	var txn Transaction
	txn.Sender[1] = 3
	txn.Fee.Raw = 1234
	txid := txn.ID()
	txid2 := Txid(crypto.HashObj(txn))
	require.Equal(t, txid, txid2)

	txn.LastValid = 4321
	txid3 := txn.ID()
	txid2 = Txid(crypto.HashObj(txn))
	require.NotEqual(t, txid, txid3)
	require.Equal(t, txid3, txid2)
}

var generateFlag = flag.Bool("generate", false, "")

// running test with -generate would generate the matrix used in the test ( without the "correct" errors )
func TestWellFormedKeyRegistrationTx(t *testing.T) {
	partitiontest.PartitionTest(t)

	flag.Parse()

	// addr has no significance here other than being a normal valid address
	addr, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)

	tx := generateDummyGoNonparticpatingTransaction(addr)
	curProto := config.Consensus[protocol.ConsensusCurrentVersion]
	if !curProto.SupportBecomeNonParticipatingTransactions {
		t.Skipf("Skipping rest of test because current protocol version %v does not support become-nonparticipating transactions", protocol.ConsensusCurrentVersion)
	}

	// this tx is well-formed
	err = tx.WellFormed(SpecialAddresses{}, curProto)
	require.NoError(t, err)

	type keyRegTestCase struct {
		votePK                                    crypto.OneTimeSignatureVerifier
		selectionPK                               crypto.VRFVerifier
		stateProofPK                              merklesignature.Commitment
		voteFirst                                 basics.Round
		voteLast                                  basics.Round
		lastValid                                 basics.Round
		voteKeyDilution                           uint64
		nonParticipation                          bool
		supportBecomeNonParticipatingTransactions bool
		enableKeyregCoherencyCheck                bool
		enableStateProofKeyregCheck               bool
		err                                       error
	}
	votePKValue := crypto.OneTimeSignatureVerifier{0x7, 0xda, 0xcb, 0x4b, 0x6d, 0x9e, 0xd1, 0x41, 0xb1, 0x75, 0x76, 0xbd, 0x45, 0x9a, 0xe6, 0x42, 0x1d, 0x48, 0x6d, 0xa3, 0xd4, 0xef, 0x22, 0x47, 0xc4, 0x9, 0xa3, 0x96, 0xb8, 0x2e, 0xa2, 0x21}
	selectionPKValue := crypto.VRFVerifier{0x7, 0xda, 0xcb, 0x4b, 0x6d, 0x9e, 0xd1, 0x41, 0xb1, 0x75, 0x76, 0xbd, 0x45, 0x9a, 0xe6, 0x42, 0x1d, 0x48, 0x6d, 0xa3, 0xd4, 0xef, 0x22, 0x47, 0xc4, 0x9, 0xa3, 0x96, 0xb8, 0x2e, 0xa2, 0x21}

	stateProofPK := merklesignature.Commitment([merklesignature.MerkleSignatureSchemeRootSize]byte{1})
	maxValidPeriod := config.Consensus[protocol.ConsensusCurrentVersion].MaxKeyregValidPeriod

	runTestCase := func(testCase keyRegTestCase) error {

		tx.KeyregTxnFields.VotePK = testCase.votePK
		tx.KeyregTxnFields.SelectionPK = testCase.selectionPK
		tx.KeyregTxnFields.VoteFirst = testCase.voteFirst
		tx.KeyregTxnFields.VoteLast = testCase.voteLast
		tx.KeyregTxnFields.VoteKeyDilution = testCase.voteKeyDilution
		tx.KeyregTxnFields.Nonparticipation = testCase.nonParticipation
		tx.LastValid = testCase.lastValid
		tx.KeyregTxnFields.StateProofPK = testCase.stateProofPK

		curProto.SupportBecomeNonParticipatingTransactions = testCase.supportBecomeNonParticipatingTransactions
		curProto.EnableKeyregCoherencyCheck = testCase.enableKeyregCoherencyCheck
		curProto.EnableStateProofKeyregCheck = testCase.enableStateProofKeyregCheck
		curProto.MaxKeyregValidPeriod = maxValidPeriod // TODO: remove this when MaxKeyregValidPeriod is in CurrentVersion
		return tx.WellFormed(SpecialAddresses{}, curProto)
	}

	if *generateFlag == true {
		fmt.Printf("keyRegTestCases := []keyRegTestCase{\n")
		idx := 0
		for _, votePK := range []crypto.OneTimeSignatureVerifier{{}, votePKValue} {
			for _, selectionPK := range []crypto.VRFVerifier{{}, selectionPKValue} {
				for _, voteFirst := range []basics.Round{basics.Round(0), basics.Round(5)} {
					for _, voteLast := range []basics.Round{basics.Round(0), basics.Round(10)} {
						for _, lastValid := range []basics.Round{basics.Round(4), basics.Round(3)} {
							for _, voteKeyDilution := range []uint64{0, 10000} {
								for _, nonParticipation := range []bool{false, true} {
									for _, supportBecomeNonParticipatingTransactions := range []bool{false, true} {
										for _, enableKeyregCoherencyCheck := range []bool{false, true} {
											for _, enableStateProofKeyregCheck := range []bool{false, true} {
												outcome := runTestCase(keyRegTestCase{
													votePK,
													selectionPK,
													stateProofPK,
													voteFirst,
													voteLast,
													lastValid,
													voteKeyDilution,
													nonParticipation,
													supportBecomeNonParticipatingTransactions,
													enableKeyregCoherencyCheck,
													enableStateProofKeyregCheck,
													nil})
												errStr := "nil"
												switch outcome {
												case errKeyregTxnUnsupportedSwitchToNonParticipating:
													errStr = "errKeyregTxnUnsupportedSwitchToNonParticipating"
												case errKeyregTxnGoingOnlineWithNonParticipating:
													errStr = "errKeyregTxnGoingOnlineWithNonParticipating"
												case errKeyregTxnNonCoherentVotingKeys:
													errStr = "errKeyregTxnNonCoherentVotingKeys"
												case errKeyregTxnOfflineTransactionHasVotingRounds:
													errStr = "errKeyregTxnOfflineTransactionHasVotingRounds"
												case errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound:
													errStr = "errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound"
												case errKeyregTxnGoingOnlineWithZeroVoteLast:
													errStr = "errKeyregTxnGoingOnlineWithZeroVoteLast"
												case errKeyregTxnGoingOnlineWithNonParticipating:
													errStr = "errKeyregTxnGoingOnlineWithNonParticipating"
												case errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid:
													errStr = "errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid"
												default:
													require.Nil(t, outcome)

												}
												s := "/* %3d */ keyRegTestCase{votePK:"
												if votePK == votePKValue {
													s += "votePKValue"
												} else {
													s += "crypto.OneTimeSignatureVerifier{}"
												}
												s += ", selectionPK:"
												if selectionPK == selectionPKValue {
													s += "selectionPKValue"
												} else {
													s += "crypto.VRFVerifier{}"
												}
												s = fmt.Sprintf("%s, voteFirst:basics.Round(%2d), voteLast:basics.Round(%2d), lastValid:basics.Round(%2d), voteKeyDilution: %5d, nonParticipation: %v,supportBecomeNonParticipatingTransactions:%v, enableKeyregCoherencyCheck:%v, err:%s},\n",
													s, voteFirst, voteLast, lastValid, voteKeyDilution, nonParticipation, supportBecomeNonParticipatingTransactions, enableKeyregCoherencyCheck, errStr)
												fmt.Printf(s, idx)
												idx++
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
		fmt.Printf("}\n")
		return
	}
	keyRegTestCases := []keyRegTestCase{
		/*   0 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*   1 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: nil},
		/*   2 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*   3 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/*   4 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*   5 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*   6 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*   7 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/*   8 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*   9 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  10 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  11 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  12 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  13 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  14 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  15 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  16 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  17 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: nil},
		/*  18 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  19 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/*  20 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  21 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  22 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  23 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/*  24 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  25 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  26 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  27 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  28 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  29 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  30 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  31 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  32 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  33 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  34 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  35 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  36 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  37 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  38 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  39 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  40 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  41 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  42 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  43 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  44 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  45 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  46 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  47 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  48 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  49 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  50 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  51 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  52 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  53 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  54 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  55 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  56 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  57 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  58 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  59 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  60 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  61 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  62 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  63 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/*  64 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  65 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  66 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  67 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  68 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  69 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  70 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  71 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  72 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  73 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  74 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  75 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  76 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  77 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  78 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  79 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  80 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  81 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  82 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  83 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  84 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  85 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  86 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  87 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  88 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  89 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  90 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  91 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  92 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/*  93 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  94 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  95 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/*  96 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/*  97 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/*  98 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/*  99 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 100 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 101 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 102 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 103 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 104 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 105 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 106 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 107 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 108 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 109 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 110 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 111 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 112 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 113 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 114 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 115 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 116 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 117 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 118 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 119 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnOfflineTransactionHasVotingRounds},
		/* 120 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 121 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 122 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 123 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 124 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 125 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 126 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 127 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 128 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 129 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 130 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 131 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 132 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 133 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 134 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 135 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 136 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 137 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 138 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 139 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 140 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 141 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 142 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 143 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 144 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 145 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 146 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 147 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 148 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 149 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 150 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 151 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 152 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 153 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 154 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 155 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 156 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 157 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 158 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 159 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 160 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 161 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 162 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 163 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 164 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 165 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 166 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 167 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 168 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 169 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 170 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 171 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 172 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 173 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 174 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 175 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 176 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 177 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 178 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 179 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 180 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 181 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 182 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 183 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 184 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 185 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 186 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 187 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 188 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 189 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 190 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 191 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 192 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 193 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 194 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 195 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 196 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 197 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 198 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 199 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 200 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 201 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 202 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 203 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 204 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 205 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 206 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 207 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 208 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 209 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 210 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 211 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 212 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 213 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 214 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 215 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 216 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 217 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 218 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 219 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 220 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 221 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 222 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 223 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 224 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 225 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 226 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 227 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 228 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 229 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 230 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 231 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 232 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 233 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 234 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 235 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 236 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 237 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 238 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 239 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 240 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 241 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 242 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 243 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 244 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 245 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 246 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 247 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 248 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 249 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 250 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 251 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 252 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 253 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 254 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 255 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 256 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 257 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 258 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 259 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 260 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 261 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 262 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 263 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 264 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 265 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 266 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 267 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 268 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 269 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 270 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 271 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 272 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 273 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 274 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 275 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 276 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 277 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 278 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 279 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 280 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 281 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 282 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 283 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 284 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 285 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 286 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 287 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 288 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 289 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 290 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 291 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 292 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 293 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 294 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 295 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 296 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 297 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 298 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 299 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 300 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 301 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 302 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 303 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 304 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 305 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 306 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 307 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 308 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 309 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 310 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 311 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 312 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 313 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 314 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 315 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 316 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 317 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 318 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 319 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 320 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 321 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 322 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 323 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 324 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 325 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 326 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 327 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 328 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 329 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 330 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 331 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 332 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 333 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 334 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 335 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 336 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 337 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 338 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 339 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 340 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 341 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 342 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 343 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 344 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 345 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 346 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 347 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 348 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 349 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 350 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 351 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 352 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 353 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 354 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 355 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 356 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 357 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 358 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 359 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 360 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 361 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 362 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 363 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 364 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 365 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 366 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 367 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 368 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 369 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 370 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 371 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 372 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 373 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 374 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 375 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 376 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 377 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 378 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 379 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 380 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 381 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 382 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 383 */ {votePK: votePKValue, selectionPK: crypto.VRFVerifier{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 384 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 385 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 386 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 387 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 388 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 389 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 390 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 391 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 392 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 393 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 394 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 395 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 396 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 397 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 398 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 399 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 400 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 401 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 402 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 403 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 404 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 405 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 406 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 407 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 408 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 409 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 410 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 411 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 412 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 413 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 414 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 415 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithZeroVoteLast},
		/* 416 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 417 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 418 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 419 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 420 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 421 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 422 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 423 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 424 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 425 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: nil},
		/* 426 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 427 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/* 428 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 429 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 430 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 431 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 432 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 433 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 434 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 435 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 436 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 437 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 438 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 439 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 440 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 441 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: nil},
		/* 442 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 443 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/* 444 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 445 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 446 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 447 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(0), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 448 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 449 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 450 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 451 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 452 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 453 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 454 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 455 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 456 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 457 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 458 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 459 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 460 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 461 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 462 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 463 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 464 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 465 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 466 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 467 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 468 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 469 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 470 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 471 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 472 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 473 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 474 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 475 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 476 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 477 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 478 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 479 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(0), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnFirstVotingRoundGreaterThanLastVotingRound},
		/* 480 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 481 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 482 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 483 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 484 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 485 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 486 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 487 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 488 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 489 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: nil},
		/* 490 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 491 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: nil},
		/* 492 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 493 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 494 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 495 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(4), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 496 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 497 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 498 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 499 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 500 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 501 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 502 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 503 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnNonCoherentVotingKeys},
		/* 504 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: nil},
		/* 505 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid},
		/* 506 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: nil},
		/* 507 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid},
		/* 508 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: false, err: errKeyregTxnUnsupportedSwitchToNonParticipating},
		/* 509 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: false, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid},
		/* 510 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, err: errKeyregTxnGoingOnlineWithNonParticipating},
		/* 511 */ {votePK: votePKValue, selectionPK: selectionPKValue, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: true, err: errKeyregTxnGoingOnlineWithFirstVoteAfterLastValid},
		/* 512 */ {votePK: votePKValue, selectionPK: selectionPKValue, stateProofPK: stateProofPK, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: false, err: errKeyregTxnNotEmptyStateProofPK},
		/* 513 */ {votePK: votePKValue, selectionPK: selectionPKValue, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: false, err: nil},
		/* 514 */ {votePK: votePKValue, selectionPK: selectionPKValue, stateProofPK: stateProofPK, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: nil},
		/* 515 */ {votePK: votePKValue, selectionPK: selectionPKValue, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: errKeyRegEmptyStateProofPK},
		/* 516 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: stateProofPK, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: errKeyregTxnNonParticipantShouldBeEmptyStateProofPK},
		/* 517 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: nil},
		/* 518 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: stateProofPK, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: errKeyregTxnOfflineShouldBeEmptyStateProofPK},
		/* 519 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: true, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: nil},
		/* 520 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(5), voteLast: basics.Round(10), lastValid: basics.Round(3), voteKeyDilution: 0, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: nil},
		/* 521 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(10), voteLast: basics.Round(10 + maxValidPeriod), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: nil},
		/* 522 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(10), voteLast: basics.Round(10000 + maxValidPeriod), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: true, err: errKeyRegTxnValidityPeriodTooLong},
		/* 523 */ {votePK: crypto.OneTimeSignatureVerifier{}, selectionPK: crypto.VRFVerifier{}, stateProofPK: merklesignature.Commitment{}, voteFirst: basics.Round(10), voteLast: basics.Round(10000 + maxValidPeriod), lastValid: basics.Round(3), voteKeyDilution: 10000, nonParticipation: false, supportBecomeNonParticipatingTransactions: true, enableKeyregCoherencyCheck: false, enableStateProofKeyregCheck: false, err: nil},
	}
	for testcaseIdx, testCase := range keyRegTestCases {
		err := runTestCase(testCase)

		require.Equalf(t, testCase.err, err, "index: %d\ntest case: %#v", testcaseIdx, testCase)
	}
}

type stateproofTxnTestCase struct {
	expectedError error

	StateProofInterval uint64
	fee                basics.MicroAlgos
	note               []byte
	group              crypto.Digest
	lease              [32]byte
	rekeyValue         basics.Address
	sender             basics.Address
}

func (s *stateproofTxnTestCase) runIsWellFormedForTestCase() error {
	curProto := config.Consensus[protocol.ConsensusCurrentVersion]
	curProto.StateProofInterval = s.StateProofInterval

	// edit txn params. wanted
	return Transaction{
		Type: protocol.StateProofTx,
		Header: Header{
			Sender:      s.sender,
			Fee:         s.fee,
			FirstValid:  0,
			LastValid:   0,
			Note:        s.note,
			GenesisID:   "",
			GenesisHash: crypto.Digest{},
			Group:       s.group,
			Lease:       s.lease,
			RekeyTo:     s.rekeyValue,
		},
		StateProofTxnFields: StateProofTxnFields{},
	}.WellFormed(SpecialAddresses{}, curProto)
}

func TestWellFormedStateProofTxn(t *testing.T) {
	partitiontest.PartitionTest(t)
	// want to create different Txns, run on all of these cases the check, and have an expected result
	cases := []stateproofTxnTestCase{
		/* 0 */ {expectedError: errStateProofNotSupported}, // StateProofInterval == 0 leads to error
		/* 1 */ {expectedError: errBadSenderInStateProofTxn, StateProofInterval: 256, sender: basics.Address{1, 2, 3, 4}},
		/* 2 */ {expectedError: errFeeMustBeZeroInStateproofTxn, StateProofInterval: 256, sender: StateProofSender, fee: basics.MicroAlgos{Raw: 1}},
		/* 3 */ {expectedError: errNoteMustBeEmptyInStateproofTxn, StateProofInterval: 256, sender: StateProofSender, note: []byte{1, 2, 3}},
		/* 4 */ {expectedError: errGroupMustBeZeroInStateproofTxn, StateProofInterval: 256, sender: StateProofSender, group: crypto.Digest{1, 2, 3}},
		/* 5 */ {expectedError: errRekeyToMustBeZeroInStateproofTxn, StateProofInterval: 256, sender: StateProofSender, rekeyValue: basics.Address{1, 2, 3, 4}},
		/* 6 */ {expectedError: errLeaseMustBeZeroInStateproofTxn, StateProofInterval: 256, sender: StateProofSender, lease: [32]byte{1, 2, 3, 4}},
		/* 7 */ {expectedError: nil, StateProofInterval: 256, fee: basics.MicroAlgos{Raw: 0}, note: nil, group: crypto.Digest{}, lease: [32]byte{}, rekeyValue: basics.Address{}, sender: StateProofSender},
	}
	for i, testCase := range cases {
		cpyTestCase := testCase
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, cpyTestCase.expectedError, cpyTestCase.runIsWellFormedForTestCase())
		})
	}
}

func TestStateProofTxnShouldBeZero(t *testing.T) {
	partitiontest.PartitionTest(t)

	addr1, err := basics.UnmarshalChecksumAddress("NDQCJNNY5WWWFLP4GFZ7MEF2QJSMZYK6OWIV2AQ7OMAVLEFCGGRHFPKJJA")
	require.NoError(t, err)

	curProto := config.Consensus[protocol.ConsensusCurrentVersion]
	curProto.StateProofInterval = 256
	txn := Transaction{
		Type: protocol.PaymentTx,
		Header: Header{
			Sender:      addr1,
			Fee:         basics.MicroAlgos{Raw: 100},
			FirstValid:  0,
			LastValid:   0,
			Note:        []byte{0, 1},
			GenesisID:   "",
			GenesisHash: crypto.Digest{},
		},
		StateProofTxnFields: StateProofTxnFields{},
	}

	const erroMsg = "type pay has non-zero fields for type stpf"
	txn.StateProofType = 1
	err = txn.WellFormed(SpecialAddresses{}, curProto)
	require.Error(t, err)
	require.Contains(t, err.Error(), erroMsg)

	txn.StateProofType = 0
	txn.Message = stateproofmsg.Message{FirstAttestedRound: 1}
	err = txn.WellFormed(SpecialAddresses{}, curProto)
	require.Error(t, err)
	require.Contains(t, err.Error(), erroMsg)

	txn.Message = stateproofmsg.Message{}
	txn.StateProof = stateproof.StateProof{SignedWeight: 100}
	err = txn.WellFormed(SpecialAddresses{}, curProto)
	require.Error(t, err)
	require.Contains(t, err.Error(), erroMsg)

	txn.StateProof = stateproof.StateProof{}
	txn.Message.LastAttestedRound = 512
	err = txn.WellFormed(SpecialAddresses{}, curProto)
	require.Error(t, err)
	require.Contains(t, err.Error(), erroMsg)

	txn.Message.LastAttestedRound = 0
	err = txn.WellFormed(SpecialAddresses{}, curProto)
	require.NoError(t, err)
}
