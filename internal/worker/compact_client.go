package worker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"time"

	"boon/internal/protocol"
)

// CompactClient 紧凑协议客户端
type CompactClient struct {
	baseURL    string
	httpClient *http.Client
	batchSize  int64
}

// NewCompactClient 创建客户端
func NewCompactClient(baseURL string, batchSize int64) *CompactClient {
	return &CompactClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		batchSize: batchSize,
	}
}

// FetchTemplate 获取任务模板
func (c *CompactClient) FetchTemplate(jobID int64) (*TaskTemplate, error) {
	url := fmt.Sprintf("%s/api/template?job=%d", c.baseURL, jobID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	pt, err := protocol.DecodeTemplate(data)
	if err != nil {
		return nil, err
	}

	return &TaskTemplate{
		JobID:      pt.JobID,
		Words:      pt.Words,
		UnknownPos: pt.UnknownPos,
	}, nil
}

// FetchTask 获取任务
func (c *CompactClient) FetchTask(workerID string) (*protocol.CompactTask, error) {
	url := fmt.Sprintf("%s/api/task/fetch", c.baseURL)

	// 请求体: workerID(len+data) + batchSize(8)
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(len(workerID)))
	buf.WriteString(workerID)
	binary.Write(buf, binary.BigEndian, c.batchSize)

	resp, err := c.httpClient.Post(url, "application/octet-stream", buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return protocol.DecodeCompactTask(data)
}

// SubmitResult 提交结果
func (c *CompactClient) SubmitResult(workerID string, result *protocol.CompactResult) error {
	url := fmt.Sprintf("%s/api/task/submit", c.baseURL)

	// 请求体: workerID(len+data) + result
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(len(workerID)))
	buf.WriteString(workerID)
	buf.Write(result.Encode())

	resp, err := c.httpClient.Post(url, "application/octet-stream", buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}
