package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"log"
)

func serializeMetadataStruct(metadata dirEntMetadata) ([]byte, error) {
	var metadataBuf bytes.Buffer

	encoder := gob.NewEncoder(&metadataBuf)
	if err := encoder.Encode(metadata); err != nil {
		log.Println("serializeMetadataStruct: gob.encoder.Encode failed:", err)
		return nil, err
	}

	lenOfStruct := len(metadataBuf.Bytes())

	var lenOfStructBuf bytes.Buffer
	err := binary.Write(&lenOfStructBuf, binary.LittleEndian, int64(lenOfStruct))
	if err != nil {
		log.Println("serializeMetadataStruct: binary.Write failed:", err)
		return nil, err
	}

	lengthPrefixedMetadataBuf := lenOfStructBuf.Bytes()
	lengthPrefixedMetadataBuf = append(lengthPrefixedMetadataBuf, metadataBuf.Bytes()...)

	return lengthPrefixedMetadataBuf, nil
}

func deserializeMetadataStruct(blobBuf []byte) (metadataPtr *dirEntMetadata, fileContents []byte, err error) {
	blobBuffer := bytes.NewBuffer(blobBuf)

	var structSize int64
	err = binary.Read(blobBuffer, binary.LittleEndian, &structSize)
	if err != nil {
		log.Println("deserializeMetadataStruct: binary.Read failed:", err)
		return nil, nil, err
	}

	var metadata dirEntMetadata
	decoder := gob.NewDecoder(blobBuffer)
	if err := decoder.Decode(&metadata); err != nil {
		log.Println("deserializeMetadataStruct: gob.decoder.Decode failed:", err)
		return nil, nil, err
	}

	fileContents = blobBuf[8+structSize:]

	return &metadata, fileContents, nil
}
