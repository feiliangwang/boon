package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TaskTemplate 任务模板（Worker首次获取，包含词和位置信息）
type TaskTemplate struct {
	JobID      int64    // 任务ID
	Words      []string // 12个词，空字符串表示需要枚举
	UnknownPos []int    // 需要枚举的位置索引
}

// Encode 编码模板
func (t *TaskTemplate) Encode() []byte {
	buf := new(bytes.Buffer)

	// JobID (8 bytes)
	binary.Write(buf, binary.BigEndian, t.JobID)

	// UnknownPos count (1 byte)
	buf.WriteByte(byte(len(t.UnknownPos)))

	// UnknownPos (each 1 byte)
	for _, pos := range t.UnknownPos {
		buf.WriteByte(byte(pos))
	}

	// Words (12 words, each length-prefixed)
	for _, word := range t.Words {
		buf.WriteByte(byte(len(word)))
		buf.WriteString(word)
	}

	return buf.Bytes()
}

// DecodeTemplate 解码模板
func DecodeTemplate(data []byte) (*TaskTemplate, error) {
	t := &TaskTemplate{}
	buf := bytes.NewReader(data)

	// JobID
	if err := binary.Read(buf, binary.BigEndian, &t.JobID); err != nil {
		return nil, err
	}

	// UnknownPos count
	count, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	// UnknownPos
	t.UnknownPos = make([]int, count)
	for i := 0; i < int(count); i++ {
		pos, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		t.UnknownPos[i] = int(pos)
	}

	// Words (always 12)
	t.Words = make([]string, 12)
	for i := 0; i < 12; i++ {
		length, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		if length > 0 {
			word := make([]byte, length)
			if _, err := buf.Read(word); err != nil {
				return nil, err
			}
			t.Words[i] = string(word)
		} else {
			t.Words[i] = "" // 空 = 需要枚举
		}
	}

	return t, nil
}

// CompactTask 紧凑任务（32 bytes）
type CompactTask struct {
	TaskID   int64 // 任务ID
	JobID    int64 // 所属任务
	StartIdx int64 // 开始索引
	EndIdx   int64 // 结束索引（不含）
}

// Encode 编码任务（32 bytes）
func (t *CompactTask) Encode() []byte {
	buf := make([]byte, 32)
	binary.BigEndian.PutUint64(buf[0:8], uint64(t.TaskID))
	binary.BigEndian.PutUint64(buf[8:16], uint64(t.JobID))
	binary.BigEndian.PutUint64(buf[16:24], uint64(t.StartIdx))
	binary.BigEndian.PutUint64(buf[24:32], uint64(t.EndIdx))
	return buf
}

// DecodeCompactTask 解码任务
func DecodeCompactTask(data []byte) (*CompactTask, error) {
	if len(data) < 32 {
		return nil, fmt.Errorf("data too short: %d < 32", len(data))
	}

	return &CompactTask{
		TaskID:   int64(binary.BigEndian.Uint64(data[0:8])),
		JobID:    int64(binary.BigEndian.Uint64(data[8:16])),
		StartIdx: int64(binary.BigEndian.Uint64(data[16:24])),
		EndIdx:   int64(binary.BigEndian.Uint64(data[24:32])),
	}, nil
}

// CompactResult 紧凑结果
type CompactResult struct {
	TaskID  int64
	Matches []MatchData
}

// MatchData 匹配数据（28 bytes: 8 index + 20 address）
type MatchData struct {
	Index   int64
	Address []byte // 20 bytes
}

// Encode 编码结果
func (r *CompactResult) Encode() []byte {
	// 8 (taskID) + 4 (count) + N * 28 (matches)
	size := 12 + len(r.Matches)*28
	buf := make([]byte, size)

	binary.BigEndian.PutUint64(buf[0:8], uint64(r.TaskID))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(r.Matches)))

	offset := 12
	for _, m := range r.Matches {
		binary.BigEndian.PutUint64(buf[offset:offset+8], uint64(m.Index))
		copy(buf[offset+8:offset+28], m.Address)
		offset += 28
	}

	return buf
}

// DecodeCompactResult 解码结果
func DecodeCompactResult(data []byte) (*CompactResult, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("data too short: %d < 12", len(data))
	}

	r := &CompactResult{
		TaskID: int64(binary.BigEndian.Uint64(data[0:8])),
	}

	count := binary.BigEndian.Uint32(data[8:12])
	r.Matches = make([]MatchData, count)

	offset := 12
	for i := uint32(0); i < count; i++ {
		if len(data) < offset+28 {
			return nil, fmt.Errorf("incomplete match data at %d", i)
		}
		r.Matches[i].Index = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
		r.Matches[i].Address = make([]byte, 20)
		copy(r.Matches[i].Address, data[offset+8:offset+28])
		offset += 28
	}

	return r, nil
}
