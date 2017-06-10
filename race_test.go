// +build race

package goavro_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/karrick/goavro"
)

func TestRaceEncodeEncodeArray(t *testing.T) {
	codec, err := goavro.NewCodec(`{"type":"record","name":"record1","fields":[{"name":"field1","type":"array","items":"long"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	var consumers, producers sync.WaitGroup
	consumers.Add(1)
	producers.Add(2)

	done := make(chan error, 10)
	go func() {
		defer consumers.Done()
		for err := range done {
			t.Error(err)
		}
	}()

	go func() {
		defer producers.Done()
		for i := 0; i < 10000; i++ {
			if _, err := codec.BinaryFromNative(nil, map[string]interface{}{"field1": []int{i}}); err != nil {
				done <- err
				return
			}
		}
	}()

	go func() {
		defer producers.Done()
		for i := 0; i < 10000; i++ {
			rec := map[string]interface{}{
				"field1": []interface{}{i},
			}
			if _, err := codec.BinaryFromNative(nil, rec); err != nil {
				done <- err
				return
			}
		}
	}()

	producers.Wait()
	close(done)
	consumers.Wait()
}

func TestRaceEncodeEncodeRecord(t *testing.T) {
	codec, err := goavro.NewCodec(`{"type":"record","name":"record1","fields":[{"type":"long","name":"field1"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	var consumers, producers sync.WaitGroup
	consumers.Add(1)
	producers.Add(2)

	done := make(chan error, 10)
	go func() {
		defer consumers.Done()
		for err := range done {
			t.Error(err)
		}
	}()

	go func() {
		defer producers.Done()
		for i := 0; i < 10000; i++ {
			rec := map[string]interface{}{"field1": i}
			if _, err := codec.BinaryFromNative(nil, rec); err != nil {
				done <- err
				return
			}
		}
	}()

	go func() {
		defer producers.Done()
		for i := 0; i < 10000; i++ {
			rec := map[string]interface{}{"field1": i}
			if _, err := codec.BinaryFromNative(nil, rec); err != nil {
				done <- err
				return
			}
		}
	}()

	producers.Wait()
	close(done)
	consumers.Wait()
}

func TestRaceCodecConstructionDecode(t *testing.T) {
	codec, err := goavro.NewCodec(`{"type": "long"}`)
	if err != nil {
		t.Fatal(err)
	}
	comms := make(chan []byte, 1000)

	var consumers sync.WaitGroup
	consumers.Add(1)

	done := make(chan error, 10)
	go func() {
		defer consumers.Done()
		for err := range done {
			t.Error(err)
		}
	}()

	go func() {
		defer close(comms)
		for i := 0; i < 10000; i++ {
			// Completely unrelated stateful objects were causing races
			if i%100 == 0 {
				_, _ = goavro.NewCodec(`{"type": "long"}`)
			}
			buf, err := codec.BinaryFromNative(nil, i)
			if err != nil {
				done <- err
				return
			}

			comms <- buf
		}
	}()

	go func() {
		defer close(done)
		var i int64
		for buf := range comms {
			datum, _, err := codec.NativeFromBinary(buf)
			if err != nil {
				done <- err
				return
			}
			result := datum.(int64) // Avro long values always decoded as int64
			if result != i {
				done <- fmt.Errorf("Actual: %v; Expected: %v", result, i)
				return
			}
			i++
		}
	}()

	consumers.Wait()
}

func TestRaceCodecConstruction(t *testing.T) {

	comms := make(chan []byte, 1000)
	done := make(chan error, 10)

	go func() {
		defer close(comms)
		recordSchemaJSON := `{"type": "long"}`
		codec, err := goavro.NewCodec(recordSchemaJSON)
		if err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 10000; i++ {
			buf, err := codec.BinaryFromNative(nil, i)
			if err != nil {
				done <- err
				return
			}
			comms <- buf
		}
	}()

	go func() {
		defer close(done)
		recordSchemaJSON := `{"type": "long"}`
		codec, err := goavro.NewCodec(recordSchemaJSON)
		if err != nil {
			t.Fatal(err)
		}
		var i int64
		for encoded := range comms {
			decoded, _, err := codec.NativeFromBinary(encoded)
			if err != nil {
				done <- err
				return
			}
			result := decoded.(int64) // Avro long values always decoded as int64
			if result != i {
				done <- fmt.Errorf("Actual: %v; Expected: %v", result, i)
				return
			}
			i++
		}
	}()

	err := <-done
	if err != nil {
		t.Fatal(err)
	}
}
