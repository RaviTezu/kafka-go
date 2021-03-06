package kafka

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"
	"time"
)

type createTopicsRequestV2ConfigEntry struct {
	ConfigName  string
	ConfigValue string
}

func (t createTopicsRequestV2ConfigEntry) size() int32 {
	return sizeofString(t.ConfigName) +
		sizeofString(t.ConfigValue)
}

func (t createTopicsRequestV2ConfigEntry) writeTo(w *bufio.Writer) {
	writeString(w, t.ConfigName)
	writeString(w, t.ConfigValue)
}

type createTopicsRequestV2ReplicaAssignment struct {
	Partition int32
	Replicas  int32
}

func (t createTopicsRequestV2ReplicaAssignment) size() int32 {
	return sizeofInt32(t.Partition) +
		sizeofInt32(t.Replicas)
}

func (t createTopicsRequestV2ReplicaAssignment) writeTo(w *bufio.Writer) {
	writeInt32(w, t.Partition)
	writeInt32(w, t.Replicas)
}

type createTopicsRequestV2Topic struct {
	// Topic name
	Topic string

	// NumPartitions created. -1 indicates unset.
	NumPartitions int32

	// ReplicationFactor for the topic. -1 indicates unset.
	ReplicationFactor int16

	// ReplicaAssignments among kafka brokers for this topic partitions. If this
	// is set num_partitions and replication_factor must be unset.
	ReplicaAssignments []createTopicsRequestV2ReplicaAssignment

	// ConfigEntries holds topic level configuration for topic to be set.
	ConfigEntries []createTopicsRequestV2ConfigEntry
}

func (t createTopicsRequestV2Topic) size() int32 {
	return sizeofString(t.Topic) +
		sizeofInt32(t.NumPartitions) +
		sizeofInt16(t.ReplicationFactor) +
		sizeofArray(len(t.ReplicaAssignments), func(i int) int32 { return t.ReplicaAssignments[i].size() }) +
		sizeofArray(len(t.ConfigEntries), func(i int) int32 { return t.ConfigEntries[i].size() })
}

func (t createTopicsRequestV2Topic) writeTo(w *bufio.Writer) {
	writeString(w, t.Topic)
	writeInt32(w, t.NumPartitions)
	writeInt16(w, t.ReplicationFactor)
	writeArray(w, len(t.ReplicaAssignments), func(i int) { t.ReplicaAssignments[i].writeTo(w) })
	writeArray(w, len(t.ConfigEntries), func(i int) { t.ConfigEntries[i].writeTo(w) })
}

// See http://kafka.apache.org/protocol.html#The_Messages_CreateTopics
type createTopicsRequestV2 struct {
	// Topics contains n array of single topic creation requests. Can not
	// have multiple entries for the same topic.
	Topics []createTopicsRequestV2Topic

	// Timeout ms to wait for a topic to be completely created on the
	// controller node. Values <= 0 will trigger topic creation and return immediately
	Timeout int32

	// ValidateOnly if true, the request will be validated, but the topic won
	// 't be created.
	ValidateOnly bool
}

func (t createTopicsRequestV2) size() int32 {
	return sizeofArray(len(t.Topics), func(i int) int32 { return t.Topics[i].size() }) +
		sizeofInt32(t.Timeout) +
		sizeofBool(t.ValidateOnly)
}

func (t createTopicsRequestV2) writeTo(w *bufio.Writer) {
	writeArray(w, len(t.Topics), func(i int) { t.Topics[i].writeTo(w) })
	writeInt32(w, t.Timeout)
	writeBool(w, t.ValidateOnly)
}

type createTopicsResponseV2TopicError struct {
	// Topic name
	Topic string

	// ErrorCode holds response error code
	ErrorCode int16

	// ErrorMessage holds the response error message
	ErrorMessage string
}

func (t createTopicsResponseV2TopicError) size() int32 {
	return sizeofString(t.Topic) +
		sizeofInt16(t.ErrorCode) +
		sizeofString(t.ErrorMessage)
}

func (t createTopicsResponseV2TopicError) writeTo(w *bufio.Writer) {
	writeString(w, t.Topic)
	writeInt16(w, t.ErrorCode)
	writeString(w, t.ErrorMessage)
}

func (t *createTopicsResponseV2TopicError) readFrom(r *bufio.Reader, size int) (remain int, err error) {
	if remain, err = readString(r, size, &t.Topic); err != nil {
		return
	}
	if remain, err = readInt16(r, remain, &t.ErrorCode); err != nil {
		return
	}
	if remain, err = readString(r, remain, &t.ErrorMessage); err != nil {
		return
	}
	return
}

// See http://kafka.apache.org/protocol.html#The_Messages_CreateTopics
type createTopicsResponseV2 struct {
	ThrottleTimeMS int32
	TopicErrors    []createTopicsResponseV2TopicError
}

func (t createTopicsResponseV2) size() int32 {
	return sizeofInt32(t.ThrottleTimeMS) +
		sizeofArray(len(t.TopicErrors), func(i int) int32 { return t.TopicErrors[i].size() })
}

func (t createTopicsResponseV2) writeTo(w *bufio.Writer) {
	writeInt32(w, t.ThrottleTimeMS)
	writeArray(w, len(t.TopicErrors), func(i int) { t.TopicErrors[i].writeTo(w) })
}

func (t *createTopicsResponseV2) readFrom(r *bufio.Reader, size int) (remain int, err error) {
	if remain, err = readInt32(r, size, &t.ThrottleTimeMS); err != nil {
		return
	}

	remain, err = readArrayWith(r, remain, func(r *bufio.Reader, size int) (remain int, err error) {
		var topic createTopicsResponseV2TopicError
		remain, err = (&topic).readFrom(r, size)
		t.TopicErrors = append(t.TopicErrors, topic)
		return
	})

	return
}

func TestCreateTopicsResponseV2(t *testing.T) {
	item := createTopicsResponseV2{
		ThrottleTimeMS: 1,
		TopicErrors: []createTopicsResponseV2TopicError{
			{
				Topic:        "topic",
				ErrorCode:    2,
				ErrorMessage: "topic error",
			},
		},
	}

	buf := bytes.NewBuffer(nil)
	w := bufio.NewWriter(buf)
	item.writeTo(w)
	w.Flush()

	var found createTopicsResponseV2
	remain, err := (&found).readFrom(bufio.NewReader(buf), buf.Len())
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if remain != 0 {
		t.Errorf("expected 0 remain, got %v", remain)
		t.FailNow()
	}
	if !reflect.DeepEqual(item, found) {
		t.Error("expected item and found to be the same")
		t.FailNow()
	}
}

func writeHeader(w *bufio.Writer, clientID string, apiKey apiKey, apiVersion apiVersion, correlationID, size int32) {
	h := requestHeader{
		ApiKey:        int16(apiKey),
		ApiVersion:    int16(apiVersion),
		CorrelationID: correlationID,
		ClientID:      clientID,
	}
	h.Size = h.size() - 4 + size

	// write message
	h.writeTo(w)
}

func (c *Conn) createTopics(request createTopicsRequestV2) (createTopicsResponseV2, error) {
	var response createTopicsResponseV2

	err := c.readOperation(
		func(deadline time.Time, id int32) error {
			w := &c.wbuf
			writeHeader(w, c.clientID, createTopicsRequest, v2, id, request.size())
			request.writeTo(w)
			return w.Flush()
		},
		func(deadline time.Time, size int) error {
			return expectZeroSize(func() (remain int, err error) {
				return (&response).readFrom(&c.rbuf, size)
			}())
		},
	)
	if err != nil {
		return response, err
	}
	for _, tr := range response.TopicErrors {
		if tr.ErrorCode != 0 {
			return response, Error(tr.ErrorCode)
		}
	}

	return response, nil
}
