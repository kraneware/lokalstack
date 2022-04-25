package lokalstack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/google/uuid"

	"github.com/aws/aws-xray-sdk-go/strategy/sampling"
	"github.com/aws/aws-xray-sdk-go/xray"
)

// NewTestDaemon https://github.com/aws/aws-xray-sdk-go/blob/v1.0.0/xray/util_test.go
func NewTestDaemon() (ctx context.Context, d *TestDaemon) {
	c := make(chan *result, 200)
	var (
		conn net.PacketConn
		err  error
	)
	conn, err = net.ListenPacket("udp", "127.0.0.1:0")

	if err == nil {
		var cancel context.CancelFunc

		ctx, cancel = context.WithCancel(context.Background())
		d = &TestDaemon{
			ch:     c,
			conn:   conn,
			ctx:    ctx,
			cancel: cancel,
		}

		var emitter *xray.DefaultEmitter
		emitter, err = xray.NewDefaultEmitter(conn.LocalAddr().(*net.UDPAddr))

		if err == nil {
			ctx, err = xray.ContextWithConfig(ctx, xray.Config{
				Emitter:                emitter,
				DaemonAddr:             conn.LocalAddr().String(),
				ServiceVersion:         "TestVersion",
				SamplingStrategy:       &TestSamplingStrategy{},
				ContextMissingStrategy: &TestContextMissingStrategy{},
				StreamingStrategy:      &TestStreamingStrategy{},
			})
			if err == nil {
				// adding lambda context
				lambdaContext := lambdacontext.NewContext(ctx, &lambdacontext.LambdaContext{
					AwsRequestID:  uuid.New().String(),
					Identity:      lambdacontext.CognitoIdentity{},
					ClientContext: lambdacontext.ClientContext{},
				})
				ctx, _ = xray.BeginSegment(lambdaContext, uuid.New().String())
			}

			go d.run(c)
		} else {
			panic(errors.Wrapf(err, "xray: failed to created emitter:"))
		}
	} else if _, err = net.ListenPacket("udp6", "[::1]:0"); err != nil {
		panic(errors.Wrapf(err, "xray: failed to listen:"))
	}

	return // nolint:nakedret
}

func (td *TestDaemon) Close() {
	td.closeOnce.Do(func() {
		td.cancel()
		td.conn.Close()
	})
}

func (td *TestDaemon) run(c chan *result) {
	buffer := make([]byte, 64*1024)
	for {
		n, _, err := td.conn.ReadFrom(buffer)
		if err != nil {
			select {
			case c <- &result{nil, err}:
			case <-td.ctx.Done():
				return
			}
			continue
		}

		idx := bytes.IndexByte(buffer, '\n')
		buffered := buffer[idx+1 : n]

		seg := &xray.Segment{}
		err = json.Unmarshal(buffered, &seg)
		if err != nil {
			select {
			case c <- &result{nil, err}:
			case <-td.ctx.Done():
				return
			}
			continue
		}

		seg.Sampled = true
		select {
		case c <- &result{seg, nil}:
		case <-td.ctx.Done():
			return
		}
	}
}

func (td *TestDaemon) Recv() (*xray.Segment, error) {
	ctx, cancel := context.WithTimeout(td.ctx, 500*time.Millisecond)
	defer cancel()
	select {
	case r := <-td.ch:
		return r.Segment, r.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (tss *TestSamplingStrategy) ShouldTrace(request *sampling.Request) *sampling.Decision {
	return &sampling.Decision{
		Sample: true,
	}
}

func (cms *TestContextMissingStrategy) ContextMissing(v interface{}) {
	fmt.Printf("Test ContextMissing Strategy %v\n", v)
}

func (sms *TestStreamingStrategy) RequiresStreaming(seg *xray.Segment) bool {
	return false
}

func (sms *TestStreamingStrategy) StreamCompletedSubsegments(seg *xray.Segment) [][]byte {
	var test [][]byte
	return test
}

type TestDaemon struct {
	ch        <-chan *result
	conn      net.PacketConn
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}
type result struct {
	Segment *xray.Segment
	Error   error
}
type TestSamplingStrategy struct{}
type TestStreamingStrategy struct{}
type TestExceptionFormattingStrategy struct{}
type TestContextMissingStrategy struct{}
type TestEmitter struct{}
