/*
Copyright 2019-2020 vChain, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package database

import (
	"crypto/sha256"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/codenotary/immudb/embedded/store"
	"github.com/codenotary/immudb/pkg/api/schema"
	"github.com/codenotary/immudb/pkg/logger"
	"github.com/dgraph-io/badger/v2/pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var kvs = []*schema.KeyValue{
	{
		Key:   []byte("Alberto"),
		Value: []byte("Tomba"),
	},
	{
		Key:   []byte("Jean-Claude"),
		Value: []byte("Killy"),
	},
	{
		Key:   []byte("Franz"),
		Value: []byte("Clamer"),
	},
}

func makeDb() (DB, func()) {
	dbName := "EdithPiaf" + strconv.FormatInt(time.Now().UnixNano(), 10)
	options := DefaultOption().WithDbName(dbName).WithCorruptionChecker(false)
	db, err := NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	if err != nil {
		log.Fatalf("Error creating Db instance %s", err)
	}

	return db, func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
		if err := os.RemoveAll(options.dbRootPath); err != nil {
			log.Fatal(err)
		}
	}
}

func TestDefaultDbCreation(t *testing.T) {
	options := DefaultOption()
	db, err := NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	if err != nil {
		t.Fatalf("Error creating Db instance %s", err)
	}

	defer func() {
		db.Close()
		time.Sleep(1 * time.Second)
		os.RemoveAll(options.GetDbRootPath())
	}()

	dbPath := path.Join(options.GetDbRootPath(), options.GetDbName())
	if _, err = os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Db dir not created")
	}

	_, err = os.Stat(path.Join(options.GetDbRootPath()))
	if os.IsNotExist(err) {
		t.Fatalf("Data dir not created")
	}
}

func TestDbCreationInAlreadyExistentDirectories(t *testing.T) {
	options := DefaultOption().WithDbRootPath("Paris").WithDbName("EdithPiaf")
	defer os.RemoveAll(options.GetDbRootPath())

	err := os.MkdirAll(options.GetDbRootPath(), os.ModePerm)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(options.GetDbRootPath(), options.GetDbName()), os.ModePerm)
	require.NoError(t, err)

	_, err = NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	require.Error(t, err)
}

func TestDbCreationInInvalidDirectory(t *testing.T) {
	options := DefaultOption().WithDbRootPath("/?").WithDbName("EdithPiaf")
	defer os.RemoveAll(options.GetDbRootPath())

	_, err := NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	require.Error(t, err)
}

func TestDbCreation(t *testing.T) {
	options := DefaultOption().WithDbName("EdithPiaf").WithDbRootPath("Paris")
	db, err := NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	if err != nil {
		t.Fatalf("Error creating Db instance %s", err)
	}

	defer func() {
		db.Close()
		time.Sleep(1 * time.Second)
		os.RemoveAll(options.GetDbRootPath())
	}()

	dbPath := path.Join(options.GetDbRootPath(), options.GetDbName())
	if _, err = os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Db dir not created")
	}

	_, err = os.Stat(options.GetDbRootPath())
	if os.IsNotExist(err) {
		t.Fatalf("Data dir not created")
	}
}

func TestOpenWithMissingDBDirectories(t *testing.T) {
	options := DefaultOption().WithDbRootPath("Paris")
	_, err := OpenDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	require.Error(t, err)
}

func TestOpenDb(t *testing.T) {
	options := DefaultOption().WithDbName("EdithPiaf").WithDbRootPath("Paris")
	db, err := NewDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	if err != nil {
		t.Fatalf("Error creating Db instance %s", err)
	}

	err = db.Close()
	if err != nil {
		t.Fatalf("Error closing store %s", err)
	}

	db, err = OpenDb(options, logger.NewSimpleLogger("immudb ", os.Stderr))
	if err != nil {
		t.Fatalf("Error opening database %s", err)
	}

	db.Close()
	time.Sleep(1 * time.Second)
	os.RemoveAll(options.GetDbRootPath())
}

func TestDbSetGet(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	var trustedAlh [sha256.Size]byte
	var trustedIndex uint64

	for i, kv := range kvs {
		txMetadata, err := db.Set(&schema.SetRequest{KVs: []*schema.KeyValue{kv}})
		require.NoError(t, err)
		require.Equal(t, uint64(i+1), txMetadata.Id)

		if i == 0 {
			alh := schema.TxMetadataFrom(txMetadata).Alh()
			copy(trustedAlh[:], alh[:])
			trustedIndex = 1
		}

		keyReq := &schema.KeyRequest{Key: kv.Key, FromTx: int64(txMetadata.Id)}

		item, err := db.Get(keyReq)
		require.NoError(t, err)
		require.Equal(t, kv.Key, item.Key)
		require.Equal(t, kv.Value, item.Value)

		vitem, err := db.VerifiableGet(&schema.VerifiableGetRequest{
			KeyRequest:  keyReq,
			ProveFromTx: int64(trustedIndex),
		})
		require.NoError(t, err)
		require.Equal(t, kv.Key, vitem.Item.Key)
		require.Equal(t, kv.Value, vitem.Item.Value)

		inclusionProof := schema.InclusionProofFrom(vitem.InclusionProof)
		dualProof := schema.DualProofFrom(vitem.VerifiableTx.DualProof)

		var eh [sha256.Size]byte
		var sourceID, targetID uint64
		var sourceAlh, targetAlh [sha256.Size]byte

		if trustedIndex <= vitem.Item.Tx {
			copy(eh[:], dualProof.TargetTxMetadata.Eh[:])
			sourceID = trustedIndex
			sourceAlh = trustedAlh
			targetID = vitem.Item.Tx
			targetAlh = dualProof.TargetTxMetadata.Alh()
		} else {
			copy(eh[:], dualProof.SourceTxMetadata.Eh[:])
			sourceID = vitem.Item.Tx
			sourceAlh = dualProof.SourceTxMetadata.Alh()
			targetID = trustedIndex
			targetAlh = trustedAlh
		}

		verifies := store.VerifyInclusion(
			inclusionProof,
			&store.KV{Key: vitem.Item.Key, Value: vitem.Item.Value},
			eh,
		)
		require.True(t, verifies)

		verifies = store.VerifyDualProof(
			dualProof,
			sourceID,
			targetID,
			sourceAlh,
			targetAlh,
		)
		require.True(t, verifies)
	}

	_, err := db.Get(&schema.KeyRequest{Key: []byte{}})
	require.Error(t, err)
}

func TestCurrentImmutableState(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	for ind, val := range kvs {
		txMetadata, err := db.Set(&schema.SetRequest{KVs: []*schema.KeyValue{{Key: val.Key, Value: val.Value}}})
		require.NoError(t, err)
		require.Equal(t, uint64(ind+1), txMetadata.Id)

		time.Sleep(1 * time.Second)

		state, err := db.CurrentImmutableState()
		require.NoError(t, err)
		require.Equal(t, uint64(ind+1), state.TxId)
	}
}

func TestSafeSetGet(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	state, err := db.CurrentImmutableState()
	require.NoError(t, err)

	kv := []*schema.VerifiableSetRequest{
		{
			SetRequest: &schema.SetRequest{
				KVs: []*schema.KeyValue{
					{
						Key:   []byte("Alberto"),
						Value: []byte("Tomba"),
					},
				},
			},
			ProveFromTx: int64(state.TxId),
		},
		{
			SetRequest: &schema.SetRequest{
				KVs: []*schema.KeyValue{
					{
						Key:   []byte("Jean-Claude"),
						Value: []byte("Killy"),
					},
				},
			},
			ProveFromTx: int64(state.TxId),
		},
		{
			SetRequest: &schema.SetRequest{
				KVs: []*schema.KeyValue{
					{
						Key:   []byte("Franz"),
						Value: []byte("Clamer"),
					},
				},
			},
			ProveFromTx: int64(state.TxId),
		},
	}

	for ind, val := range kv {
		vtx, err := db.VerifiableSet(val)
		require.NoError(t, err)
		require.NotNil(t, vtx)

		vit, err := db.VerifiableGet(&schema.VerifiableGetRequest{
			KeyRequest: &schema.KeyRequest{
				Key:    val.SetRequest.KVs[0].Key,
				FromTx: int64(vtx.Tx.Metadata.Id),
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint64(ind+1), vit.Item.Tx)
	}
}

func TestSetGetAll(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	kvs := []*schema.KeyValue{
		{
			Key:   []byte("Alberto"),
			Value: []byte("Tomba"),
		},
		{
			Key:   []byte("Jean-Claude"),
			Value: []byte("Killy"),
		},
		{
			Key:   []byte("Franz"),
			Value: []byte("Clamer"),
		},
	}

	txMetadata, err := db.Set(&schema.SetRequest{KVs: kvs})
	require.NoError(t, err)
	require.Equal(t, uint64(1), txMetadata.Id)

	itList, err := db.GetAll(&schema.KeyListRequest{
		Keys: [][]byte{
			[]byte("Alberto"),
			[]byte("Jean-Claude"),
			[]byte("Franz"),
		},
		FromTx: int64(txMetadata.Id),
	})
	require.NoError(t, err)

	for ind, val := range itList.Items {
		require.Equal(t, kvs[ind].Value, val.Value)
	}
}

func TestTxByID(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	for ind, val := range kvs {
		txMetadata, err := db.Set(&schema.SetRequest{KVs: []*schema.KeyValue{{Key: val.Key, Value: val.Value}}})
		require.NoError(t, err)
		require.Equal(t, uint64(ind+1), txMetadata.Id)
	}

	_, err := db.TxByID(&schema.TxRequest{Tx: uint64(1)})
	require.NoError(t, err)
}

func TestVerifiableTxByID(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	for _, val := range kvs {
		_, err := db.Set(&schema.SetRequest{KVs: []*schema.KeyValue{{Key: val.Key, Value: val.Value}}})
		require.NoError(t, err)
	}

	_, err := db.VerifiableTxByID(&schema.VerifiableTxRequest{
		Tx:          uint64(1),
		ProveFromTx: 0,
	})
	require.NoError(t, err)
}

func TestHistory(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	for _, val := range kvs {
		_, err := db.Set(&schema.SetRequest{KVs: []*schema.KeyValue{{Key: val.Key, Value: val.Value}}})
		require.NoError(t, err)
	}

	time.Sleep(1 * time.Millisecond)

	inc, err := db.History(&schema.HistoryRequest{
		Key: kvs[0].Key,
	})
	require.NoError(t, err)

	for _, val := range inc.Items {
		require.Equal(t, kvs[0].Value, val.Value)
	}
}

func TestHealth(t *testing.T) {
	db, closer := makeDb()
	defer closer()
	h, err := db.Health(&emptypb.Empty{})
	if err != nil {
		t.Fatalf("health error %s", err)
	}
	if !h.GetStatus() {
		t.Fatalf("Health, expected %v, got %v", true, h.GetStatus())
	}
}

/*
func TestReference(t *testing.T) {
	db, closer := makeDb()
	defer closer()
	_, err := db.Set(kvs[0])
	if err != nil {
		t.Fatalf("Reference error %s", err)
	}
	ref, err := db.Reference(&schema.ReferenceOptions{
		Reference: []byte(`tag`),
		Key:       kvs[0].Key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Index != 1 {
		t.Fatalf("Reference, expected %v, got %v", 1, ref.Index)
	}
	item, err := db.Get(&schema.Key{Key: []byte(`tag`)})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Value, kvs[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(item.Value), string(kvs[0].Value))
	}
	item, err = db.GetReference(&schema.Key{Key: []byte(`tag`)})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Value, kvs[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(item.Value), string(kvs[0].Value))
	}
}

func TestGetReference(t *testing.T) {
	db, closer := makeDb()
	defer closer()
	_, err := db.Set(kvs[0])
	if err != nil {
		t.Fatalf("Reference error %s", err)
	}
	ref, err := db.Reference(&schema.ReferenceOptions{
		Reference: []byte(`tag`),
		Key:       kvs[0].Key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Index != 1 {
		t.Fatalf("Reference, expected %v, got %v", 1, ref.Index)
	}
	item, err := db.GetReference(&schema.Key{Key: []byte(`tag`)})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Value, kvs[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(item.Value), string(kvs[0].Value))
	}
	item, err = db.GetReference(&schema.Key{Key: []byte(`tag`)})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Value, kvs[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(item.Value), string(kvs[0].Value))
	}
}

func TestZAdd(t *testing.T) {
	db, closer := makeDb()
	defer closer()
	_, _ = db.Set(&schema.KeyValue{
		Key:   []byte(`key`),
		Value: []byte(`val`),
	})

	ref, err := db.ZAdd(&schema.ZAddOptions{
		Key:   []byte(`key`),
		Score: &schema.Score{Score: float64(1)},
		Set:   []byte(`mySet`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if ref.Index != 1 {
		t.Fatalf("Reference, expected %v, got %v", 1, ref.Index)
	}
	item, err := db.ZScan(&schema.ZScanOptions{
		Set:     []byte(`mySet`),
		Offset:  []byte(""),
		Limit:   3,
		Reverse: false,
	})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}

	assert.Equal(t, item.Items[0].Item.Value, []byte(`val`))
}
*/

/*
func TestScan(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	_, err := db.Set(kv[0])
	if err != nil {
		t.Fatalf("set error %s", err)
	}
	ref, err := db.ZAdd(&schema.ZAddOptions{
		Key:   kv[0].Key,
		Score: &schema.Score{Score: float64(3)},
		Set:   kv[0].Value,
	})
	if err != nil {
		t.Fatalf("zadd error %s", err)
	}
	if ref.Index != 1 {
		t.Fatalf("Reference, expected %v, got %v", 1, ref.Index)
	}

	it, err := db.SafeZAdd(&schema.SafeZAddOptions{
		Zopts: &schema.ZAddOptions{
			Key:   kv[0].Key,
			Score: &schema.Score{Score: float64(0)},
			Set:   kv[0].Value,
		},
		RootIndex: &schema.Index{
			Index: 0,
		},
	})
	if err != nil {
		t.Fatalf("SafeZAdd error %s", err)
	}
	if it.InclusionProof.I != 2 {
		t.Fatalf("SafeZAdd index, expected %v, got %v", 2, it.InclusionProof.I)
	}

	item, err := db.Scan(&schema.ScanOptions{
		Offset: nil,
		Deep:   false,
		Limit:  1,
		Prefix: kv[0].Key,
	})

	if err != nil {
		t.Fatalf("ZScanSV  Get error %s", err)
	}
	if !bytes.Equal(item.Items[0].Value, kv[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(kv[0].Value), string(item.Items[0].Value))
	}

	scanItem, err := db.IScan(&schema.IScanOptions{
		PageNumber: 2,
		PageSize:   1,
	})
	if err != nil {
		t.Fatalf("IScan  Get error %s", err)
	}
	// reference contains also the timestamp
	key, _, _ := store.UnwrapZIndexReference(scanItem.Items[0].Value)
	if !bytes.Equal(key, kv[0].Key) {
		t.Fatalf("Reference, expected %v, got %v", string(kv[0].Key), string(scanItem.Items[0].Value))
	}
}
*/

/*

func TestCount(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	root, err := db.CurrentRoot()
	if err != nil {
		t.Error(err)
	}

	kv := []*schema.SafeSetOptions{
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Jean-Claude"),
				Value: []byte("Killy"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Franz"),
				Value: []byte("Clamer"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
	}

	for _, val := range kv {
		_, err := db.SafeSet(val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}

	// Count
	c, err := db.Count(&schema.KeyPrefix{
		Prefix: []byte("Franz"),
	})
	if err != nil {
		t.Fatalf("Error count %s", err)
	}
	if c.Count != 1 {
		t.Fatalf("Error count expected %d got %d", 1, c.Count)
	}

	// CountAll
	// for each key there's an extra entry in the db:
	// 3 entries (with different keys) + 3 extra = 6 entries in total
	countAll := db.CountAll().Count
	if countAll != 6 {
		t.Fatalf("Error CountAll expected %d got %d", 6, countAll)
	}
}
*/

/*
func TestSafeReference(t *testing.T) {
	db, closer := makeDb()
	defer closer()
	root, err := db.CurrentRoot()
	if err != nil {
		t.Error(err)
	}
	kv := []*schema.SafeSetOptions{
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
	}
	for _, val := range kv {
		_, err := db.SafeSet(val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}
	_, err = db.SafeReference(&schema.SafeReferenceOptions{
		Ro: &schema.ReferenceOptions{
			Key:       []byte("Alberto"),
			Reference: []byte("Skii"),
		},
		RootIndex: &schema.Index{
			Index: root.GetIndex(),
		},
	})
	if err != nil {
		t.Fatalf("SafeReference Error %s", err)
	}

	_, err = db.SafeReference(&schema.SafeReferenceOptions{
		Ro: &schema.ReferenceOptions{
			Key:       []byte{},
			Reference: []byte{},
		},
	})
	if err == nil {
		t.Fatalf("SafeReference expected error %s", err)
	}
}


func TestDump(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	root, err := db.CurrentRoot()
	if err != nil {
		t.Error(err)
	}

	kvs := []*schema.SafeSetOptions{
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Jean-Claude"),
				Value: []byte("Killy"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Franz"),
				Value: []byte("Clamer"),
			},
			RootIndex: &schema.Index{
				Index: root.GetIndex(),
			},
		},
	}
	for _, val := range kvs {
		_, err := db.SafeSet(val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}

	dump := &mockImmuService_DumpServer{}
	err = db.Dump(&emptypb.Empty{}, dump)
	require.NoError(t, err)
	require.Less(t, 0, len(dump.results))
}
*/

type mockImmuService_DumpServer struct {
	grpc.ServerStream
	results []*pb.KVList
}

func (_m *mockImmuService_DumpServer) Send(kvs *pb.KVList) error {
	_m.results = append(_m.results, kvs)
	return nil
}

/*
func TestDb_SetBatchAtomicOperations(t *testing.T) {
	db, closer := makeDb()
	defer closer()

	aOps := &schema.Ops{
		Operations: []*schema.Op{
			{
				Operation: &schema.Op_KVs{
					KVs: &schema.KeyValue{
						Key:   []byte(`key`),
						Value: []byte(`val`),
					},
				},
			},
		},
	}

	_, err := db.ExecAllOps(aOps)

	require.NoError(t, err)
}
*/