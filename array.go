package goavro

import (
	"fmt"
	"reflect"
)

func (st symtab) makeArrayCodec(enclosingNamespace string, schema interface{}) (*codec, error) {
	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot create array codec: expected: map[string]interface{}; received: %T", schema)
	}
	// array type must have items
	v, ok := schemaMap["items"]
	if !ok {
		return nil, fmt.Errorf("cannot create array codec: ought to have items key")
	}
	valuesCodec, err := st.buildCodec(enclosingNamespace, v)
	if err != nil {
		return nil, fmt.Errorf("cannot create array codec: cannot create codec for specified items type: %s", err)
	}

	return &codec{
		name: "array (FIXME)",
		decoder: func(buf []byte) (interface{}, []byte, error) {
			var value interface{}
			var err error

			// NOTE: Because array values can be one or more block counts followed by their values,
			// we cannot preallocate the arrayValues slice.
			var arrayValues []interface{}

			if value, buf, err = longDecoder(buf); err != nil {
				return nil, buf, fmt.Errorf("cannot decode array: cannot decode block count: %s", err)
			}
			blockCount := value.(int64)

			for blockCount != 0 {
				if blockCount < 0 {
					// NOTE: Negative block count means following long is the block size, for which
					// we have no use.  Read its value and discard.
					blockCount = -blockCount // convert to its positive equivalent
					if _, buf, err = longDecoder(buf); err != nil {
						return nil, buf, fmt.Errorf("cannot decode array: cannot decode block size: %s", err)
					}
				}
				// Decode `blockCount` datum values from buffer
				for i := int64(0); i < blockCount; i++ {
					if value, buf, err = valuesCodec.decoder(buf); err != nil {
						return nil, buf, fmt.Errorf("cannot decode array: cannot decode item: %d; %s", i, err)
					}
					arrayValues = append(arrayValues, value)
				}
				// Decode next blockCount from buffer, because there may be more blocks
				if value, buf, err = longDecoder(buf); err != nil {
					return nil, buf, fmt.Errorf("cannot decode array: cannot decode block count: %s", err)
				}
				blockCount = value.(int64)
			}
			return arrayValues, buf, nil
		},
		encoder: func(buf []byte, datum interface{}) ([]byte, error) {
			var arrayValues []interface{}
			switch i := datum.(type) {
			case []interface{}:
				arrayValues = i
			default:
				// NOTE: If given any sort of slice, zip values to items as convenience to client.
				v := reflect.ValueOf(datum)
				if v.Kind() != reflect.Slice {
					return buf, fmt.Errorf("cannot encode array: received: %T", datum)
				}
				// NOTE: Two better alternatives to the current algorithm are:
				//   (1) mutate the reflection tuple underneath to convert the []int, for example,
				//       to []interface{}, with O(1) complexity
				//   (2) use copy builtin to zip the data items over, much like what gorrd does,
				//       with O(n) complexity, but more efficient than what's below.
				arrayValues = make([]interface{}, v.Len())
				for idx := 0; idx < v.Len(); idx++ {
					arrayValues[idx] = v.Index(idx).Interface()
				}
			}
			if len(arrayValues) > 0 {
				buf, _ = longEncoder(buf, len(arrayValues))
				for i, item := range arrayValues {
					if buf, err = valuesCodec.encoder(buf, item); err != nil {
						return buf, fmt.Errorf("cannot encode array: cannot encode item: %d; %v; %s", i, item, err)
					}
				}
			}
			return longEncoder(buf, 0)
		},
	}, nil
}