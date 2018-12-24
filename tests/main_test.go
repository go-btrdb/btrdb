package tests

import (
	"context"
	"fmt"
	mathrand "math/rand"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/require"

	"github.com/BTrDB/btrdb"
	"github.com/BTrDB/btrdb-server/bte"
)

//This will fail if ANY of the env enpoints are down
func TestConnectFast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("got connection error %v", err)
	}
	_ = db
}

//This should work if some endpoints are down
func TestConnectLong(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("got connection error %v", err)
	}
	_ = db
}

func TestSameUUID(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	sn1 := uuid.NewRandom().String()
	sn2 := uuid.NewRandom().String()
	uu := uuid.NewRandom()

	_, err = db.Create(context.Background(), uu, sn1, btrdb.OptKV("name", "foo"), nil)
	require.NoError(t, err)
	_, err = db.Create(context.Background(), uu, sn2, btrdb.OptKV("name", "foo"), nil)
	require.NotNil(t, err)
	require.EqualValues(t, bte.SameStream, btrdb.ToCodedError(err).Code)
	spew.Dump(err)
}
func TestChangedRangeSameVer(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	uu := uuid.NewRandom()
	fmt.Printf("calling create\n")
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	fmt.Printf("calling version\n")
	iver, err := stream.Version(context.Background())
	if err != nil {
		t.Fatalf("got iver error: %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	fmt.Printf("calling insert\n")
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	//Let it flush
	fmt.Printf("calling flush\n")
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	vals = make([]btrdb.RawPoint, 100)
	for i := 300; i < 400; i++ {
		vals[i-300].Time = int64(i)
		vals[i-300].Value = float64(i)
	}
	fmt.Printf("calling insert\n")
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert2 error %v", err)
	}
	fmt.Printf("calling flush\n")
	ferr = stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	fmt.Printf("calling version\n")
	ver, err := stream.Version(context.Background())
	if err != nil {
		t.Fatalf("got ver error %v", err)
	}
	if ver != 12 {
		t.Fatalf("expected two version to have happened iver=%d, ver=%d", iver, ver)
	}
	count := 0
	fmt.Printf("calling changes\n")
	cr, _, cerr := stream.Changes(context.Background(), ver, ver, 0)

	for _ = range cr {
		count++
	}
	if err := <-cerr; err != nil {
		t.Fatalf("got changed range error: %v", err)
	}
	if count != 0 {
		t.Fatalf("Did not get empty set for changed range on same version")
	}
}
func TestBigInsert(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("testbi/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := []btrdb.RawPoint{}
	for i := 0; i < 100000; i++ {
		vals = append(vals, btrdb.RawPoint{Time: int64(i * 1000000), Value: float64(i)})
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("unexpected error %v\n", err)
	}
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	time.Sleep(10 * time.Second)
	rvals, _, cerr := stream.RawValues(context.Background(), 0, 100000*1000000, btrdb.LatestVersion)
	rvall := []btrdb.RawPoint{}
	for v := range rvals {
		rvall = append(rvall, v)
	}
	if e := <-cerr; e != nil {
		t.Fatalf("unexpected error %v\n", err)
	}
	if len(rvall) != 100000 {
		t.Fatalf("only got %d points, wanted 100000", len(rvall))
	}
}
func TestChangedRangeDiffVer(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	iver, err := stream.Version(context.Background())
	if err != nil {
		t.Fatalf("got iver error: %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	//Let it flush
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	vals = make([]btrdb.RawPoint, 100)
	for i := 300; i < 400; i++ {
		vals[i-300].Time = int64(i)
		vals[i-300].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert2 error %v", err)
	}
	ferr = stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	ver, err := stream.Version(context.Background())
	if err != nil {
		t.Fatalf("got ver error %v", err)
	}
	if ver != 12 {
		t.Fatalf("expected two version to have happened iver=%d, ver=%d", iver, ver)
	}
	count := 0
	cr, _, cerr := stream.Changes(context.Background(), ver-1, ver, 0)

	for _ = range cr {
		count++
	}
	if err := <-cerr; err != nil {
		t.Fatalf("got changed range error: %v", err)
	}
	if count == 0 {
		t.Fatalf("Got empty for different version")
	}
}

func TestChangedRangeDiffVerNoFlush(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	//Let it flush
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	gver, err := stream.Version(context.Background())
	if err != nil {
		t.Fatalf("got gver error: %v", err)
	}
	vals = make([]btrdb.RawPoint, 100)
	for i := 300; i < 400; i++ {
		vals[i-300].Time = int64(i)
		vals[i-300].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert2 error %v", err)
	}
	count := 0
	cr, _, cerr := stream.Changes(context.Background(), gver, 0, 0)

	for _ = range cr {
		count++
	}
	if err := <-cerr; err != nil {
		t.Fatalf("got changed range error: %v", err)
	}
	if count == 0 {
		t.Fatalf("Got empty for different version")
	}
}

func TestAnnotationEmpty(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	ann, _, err := stream.Annotations(context.Background())
	if err != nil {
		t.Fatalf("get annotation error %v", err)
	}
	if len(ann) != 0 {
		t.Fatalf("annotationnonzero %v %x", len(ann), ann)
	}
}
func TestAnnotation(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}
	expectedAnn := ""
	for len(expectedAnn) < 100 {
		i := mathrand.Int() % utf8.MaxRune
		if utf8.ValidRune(rune(i)) {
			expectedAnn = expectedAnn + string(rune(i))
		}
	}
	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), btrdb.OptKV("ann", string(expectedAnn)))
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	ann, _, err := stream.Annotations(context.Background())
	if err != nil {
		t.Fatalf("get annotation error %v", err)
	}
	if *ann["ann"] != expectedAnn {
		t.Fatalf("annotation mismatch:\n%x\n%x", expectedAnn, ann["ann"])
	}
}

func TestListCollections(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}
	_, err = db.ListCollections(context.Background(), "")
	if err != nil {
		t.Fatalf("Unexpected list error: %v", err)
	}
}
func TestConnectDudEndpoints(t *testing.T) {
	//Internally there is a 2 second timeout for a dud endpoint, don't exceed that
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	eps := []string{"192.168.123.123:4410"}
	eps = append(eps, btrdb.EndpointsFromEnv()...)
	db, err := btrdb.Connect(ctx, eps...)
	if err != nil {
		t.Fatalf("got connection error %v", err)
	}
	_ = db
}

func TestConnectDeadline(t *testing.T) {
	//Internally there is a 2 second timeout for a dud endpoint, don't exceed that
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eps := []string{"8.8.8.8:4411", "8.8.8.8:4412"}
	eps = append(eps, btrdb.EndpointsFromEnv()...)
	db, err := btrdb.Connect(ctx, eps...)
	if err != context.DeadlineExceeded {
		t.Fatalf("got connection error %v, expected deadline exceeded", err)
	}
	_ = db
}

func TestInfo(t *testing.T) {
	//TEMP
	t.SkipNow()
	db, err := btrdb.Connect(context.Background(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	info, err := db.Info(context.Background())
	if err != nil {
		t.Fatalf("unexpected eror %v", err)
	}
	if !info.Healthy {
		t.Fatalf("Cluster is not healthy %v", err)
	}
}

func TestNilRootAfterDeleteDelete(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	valc, _, errc := stream.RawValues(context.Background(), 0, 98, btrdb.LatestVersion)
	count := 0
	for v := range valc {
		count++
		_ = v
	}
	err = <-errc
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	if count != 98 {
		t.Fatalf("Possible insert exclusion problem, got %v values expected %v", count, 98)
	}
	//Now delete it all
	ver, err := stream.DeleteRange(context.Background(), -100, 200)
	if err != nil {
		t.Fatalf("delete error %v", err)
	}
	_ = ver
	//That should be synchronous
	//now try delete again
	ver, err = stream.DeleteRange(context.Background(), -100, 200)
	if err != nil {
		t.Fatalf("delete error %v", err)
	}
	_ = ver
}

func TestNilRootAfterDeleteInsert(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	valc, _, errc := stream.RawValues(context.Background(), 0, 98, btrdb.LatestVersion)
	count := 0
	for v := range valc {
		count++
		_ = v
	}
	err = <-errc
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	if count != 98 {
		t.Fatalf("Possible insert exclusion problem, got %v values expected %v", count, 98)
	}
	//Now delete it all
	ver, err := stream.DeleteRange(context.Background(), -100, 200)
	if err != nil {
		t.Fatalf("delete error %v", err)
	}
	_ = ver
	//That should be synchronous
	//now try insert and query again
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	ferr = stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	valc, _, errc = stream.RawValues(context.Background(), 0, 98, btrdb.LatestVersion)
	count = 0
	for v := range valc {
		count++
		_ = v
	}
	err = <-errc
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	if count != 98 {
		t.Fatalf("Possible insert exclusion problem, got %v values expected %v", count, 98)
	}
}

func TestNilRootAfterDeleteQueryRaw(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	uu := uuid.NewRandom()
	stream, err := db.Create(context.Background(), uu, fmt.Sprintf("test/%x", uu[:]), btrdb.OptKV("name", "n"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := make([]btrdb.RawPoint, 100)
	for i := 0; i < 100; i++ {
		vals[i].Time = int64(i)
		vals[i].Value = float64(i)
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	valc, _, errc := stream.RawValues(context.Background(), 0, 98, btrdb.LatestVersion)
	count := 0
	for v := range valc {
		count++
		_ = v
	}
	err = <-errc
	if err != nil {
		t.Fatalf("got insert error %v", err)
	}
	if count != 98 {
		t.Fatalf("Possible insert exclusion problem, got %v values expected %v", count, 98)
	}
	//Now delete it all
	ver, err := stream.DeleteRange(context.Background(), -100, 200)
	if err != nil {
		t.Fatalf("delete error %v", err)
	}
	_ = ver
	//That should be synchronous
	//now try query again
	valc, _, errc = stream.RawValues(context.Background(), 0, 99, btrdb.LatestVersion)
	count = 0
	for v := range valc {
		count++
		_ = v
	}
	err = <-errc
	if err != nil {
		t.Fatalf("Got query error %v", err)
	}
	if count != 0 {
		t.Fatal("got unexpected count")
	}

}

func Test2kCreate(t *testing.T) {
	ctx := context.Background()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	guu := []byte(uuid.NewRandom())
	colprefix := fmt.Sprintf("ntest/%x", guu[:8])
	for k := 0; k < 2000; k++ {
		uu := uuid.NewRandom()
		col := fmt.Sprintf("%s.%03d", colprefix, k)
		str, cerr := db.Create(ctx, uu, col, btrdb.OptKV("name", fmt.Sprintf("%d", k)), nil)
		if cerr != nil {
			t.Fatalf("got create error %v", cerr)
		}
		_ = str
	}

}

func TestLookupALittle(t *testing.T) {
	ctx := context.Background()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	guu := []byte(uuid.NewRandom())
	colprefix := fmt.Sprintf("ntest/%x", guu[:8])
	then := time.Now()
	for k := 0; k < 20; k++ {
		for i := 0; i < 50; i++ {
			uu := uuid.NewRandom()
			col := fmt.Sprintf("%s.%03d", colprefix, k)
			str, cerr := db.Create(ctx, uu, col, btrdb.OptKV("name", fmt.Sprintf("%d", i), "unit", fmt.Sprintf("%d", k)), nil)
			if cerr != nil {
				t.Fatalf("got create error %v", cerr)
			}
			_ = str
		}
	}
	delta := time.Now().Sub(then)
	fmt.Printf("create took %s for %d streams (%s)\n", delta, 20*50, delta/(20*50))
	rvz, err := db.LookupStreams(ctx, colprefix, true, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rvz) != 50*20 {
		t.Fatalf("a expected %d streams, got %d", 50*20, len(rvz))
	}
	//There are I streams in this collection
	rvz, err = db.LookupStreams(ctx, colprefix+".000", false, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rvz) != 50 {
		t.Fatalf("b expected %d streams, got %d", 50, len(rvz))
	}
	//There are no collections called exactly .00
	rvz, err = db.LookupStreams(ctx, colprefix+".00", false, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rvz) != 0 {
		t.Fatalf("c expected %d streams, got %d", 0, len(rvz))
	}
	rvz, err = db.LookupStreams(ctx, colprefix, true, btrdb.M{"name": "23"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rvz) != 20 {
		t.Fatalf("d expected %d streams, got %d", 20, len(rvz))
	}
	rvz, err = db.LookupStreams(ctx, colprefix, true, btrdb.M{"name": "16", "unit": "13"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rvz) != 1 {
		t.Fatalf("e expected %d streams, got %d", 1, len(rvz))
	}
}
func TestCreate(t *testing.T) {
	ctx := context.Background()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	//A couple to make sure we hit all endpoints
	for i := 0; i < 10; i++ {
		for s := 0; s < 10; s++ {
			uu := uuid.NewRandom()
			coll := fmt.Sprintf("test/%x", uu[:])
			str, err := db.Create(ctx, uu, coll, btrdb.OptKV("name", fmt.Sprintf("%d", s)), nil)
			if err != nil {
				t.Fatalf("got create error %v", err)
			}
			_, err = str.Version(ctx)
			if err != nil {
				t.Fatalf("got error querying version %v", err)
			}
			data, verc, errc := str.RawValues(ctx, 0, 100, btrdb.LatestVersion)
			count := 0
			for d := range data {
				count++
				_ = d
			}
			err = <-errc
			ver := <-verc
			if err != nil {
				t.Fatalf("got error querying raw values on created but empty stream %v", err)
			}
			if count != 0 {
				t.Fatalf("Got values from empty stream")
			}
			if ver != 10 {
				t.Fatalf("Expected version 10, got %v", ver)
			}

			// Now check if we can query all of the streams
			tags, err := str.Tags(ctx)
			if err != nil {
				t.Fatalf("got error querying tags: %v", err)
			}

			ltags := make(map[string]string)
			for k, v := range tags {
				if v != nil {
					ltags[k] = *v
				}
			}
			s, err := db.LookupStreams(ctx, coll, false, ltags, nil)
			if err != nil {
				t.Fatalf("got error querying stream: %v", err)
			}
			if len(s) != 1 {
				t.Fatalf("expected one stream, got %d", len(s))
			}

			if s[0].UUID().String() != uu.String() {
				fmt.Printf("required: %s\n", uu.String())
				fmt.Printf("received: %s\n", s[0].UUID().String())
				t.Fatalf("UUID of queried stream doesn't match UUID of created stream")
			}
		}
	}
}

func TestObliterate(t *testing.T) {
	db, err := btrdb.Connect(context.TODO(), btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("Unexpected connection error: %v", err)
	}

	rv, err := db.LookupStreams(context.Background(), "obl.", true, btrdb.M{"name": "bar"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v\n", err)
	}
	oldrv := len(rv)

	uu := uuid.NewRandom()
	col := fmt.Sprintf("obl.%x", uu[:])
	stream, err := db.Create(context.Background(), uu, col, btrdb.OptKV("name", "bar"), nil)
	if err != nil {
		t.Fatalf("create error %v", err)
	}
	vals := []btrdb.RawPoint{}
	for i := 0; i < 100000; i++ {
		vals = append(vals, btrdb.RawPoint{Time: int64(i), Value: float64(i)})
	}
	err = stream.Insert(context.Background(), vals)
	if err != nil {
		t.Fatalf("unexpected error %v\n", err)
	}
	ferr := stream.Flush(context.Background())
	if ferr != nil {
		t.Fatalf("flush error %v", ferr)
	}
	rvals, _, cerr := stream.RawValues(context.Background(), 0, 100000, btrdb.LatestVersion)
	rvall := []btrdb.RawPoint{}
	for v := range rvals {
		rvall = append(rvall, v)
	}
	if e := <-cerr; e != nil {
		t.Fatalf("unexpected error %v\n", err)
	}
	if len(rvall) != 100000 {
		t.Fatalf("only got %d points, wanted 100000", len(rvall))
	}

	rv, err = db.LookupStreams(context.Background(), "obl.", true, btrdb.M{"name": "bar"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v\n", err)
	}
	if len(rv) != oldrv+1 {
		t.Fatalf("Expected %d results, got %d\n", oldrv+1, len(rv))
	} else {
		fmt.Printf("got expected lookup results: %d\n", len(rv))
	}

	//Now obliterate it
	err = stream.Obliterate(context.Background())
	if err != nil {
		t.Fatalf("obliterate error %v", err)
	}

	//Now try to get its anns (a lookup to backend)
	anns, aver, err := stream.Annotations(context.Background())
	if err == nil {
		t.Fatalf("queried anns successfully: %d res (aver %d)", len(anns), aver)
	} else {
		fmt.Printf("got expected anns error %v\n", err)
	}

	//Now try query it
	rvals, _, cerr = stream.RawValues(context.Background(), 0, 100000, btrdb.LatestVersion)
	rvall = []btrdb.RawPoint{}
	for v := range rvals {
		rvall = append(rvall, v)
	}
	e := <-cerr
	if e == nil {
		t.Fatalf("got no error (%d pts)\n", len(rvall))
	} else {
		fmt.Printf("got expected error %v\n", e)
	}

	//Try create with same uuid
	_, err = db.Create(context.Background(), uu, col, btrdb.OptKV("name", "bar"), nil)
	if err == nil {
		t.Fatalf("got no error creating duplicate uuid")
	} else {
		fmt.Printf("got (expected) create error: %v\n", err)
	}

	//Also try doing lookup
	rv, err = db.LookupStreams(context.Background(), "obl.", true, btrdb.M{"name": "bar"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v\n", err)
	}
	if len(rv) != oldrv {
		t.Fatalf("Expected %d results, got %d\n", oldrv, len(rv))
	} else {
		fmt.Printf("got expected lookup results: %d\n", len(rv))
	}
}

func TestTagLookup(t *testing.T) {
	ctx := context.Background()
	db, err := btrdb.Connect(ctx, btrdb.EndpointsFromEnv()...)
	if err != nil {
		t.Fatalf("connection error %v", err)
	}
	uu := uuid.NewRandom()
	col := fmt.Sprintf("ntest/%x/b", []byte(uu)[:8])

	str, cerr := db.Create(ctx, uu, col, btrdb.OptKV("name", "aval", "unit", "bval"), btrdb.OptKV("d", "dval"))
	if cerr != nil {
		t.Fatalf("got create error %v", cerr)
	}
	_ = str

	// ltags := make(map[string]*string)

	rv, err := db.LookupStreams(ctx, col, false, btrdb.M{"name": "aval", "unit": "bval"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v\n", err)
	}
	if len(rv) != 1 {
		t.Fatalf("Expected 1 result, got %d\n", len(rv))
	}
}
