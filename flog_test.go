package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"bou.ke/monkey"
	"github.com/stretchr/testify/assert"
)

type closeErrorWriter struct {
	closeCalls int
	closeErr   error
}

type writeCloseErrorWriter struct {
	writeErr   error
	closeErr   error
	closeCalls int
}

type blockingWriter struct {
	writeStarted chan struct{}
	writeRelease chan struct{}
	closeCalled  chan struct{}
	closeOnce    sync.Once
}

func (writer *blockingWriter) Write(data []byte) (int, error) {
	close(writer.writeStarted)
	<-writer.writeRelease
	return len(data), nil
}

func (writer *blockingWriter) Close() error {
	writer.closeOnce.Do(func() {
		close(writer.closeCalled)
	})
	return nil
}

func (writer *writeCloseErrorWriter) Write([]byte) (int, error) {
	return 0, writer.writeErr
}

func (writer *writeCloseErrorWriter) Close() error {
	writer.closeCalls++
	return writer.closeErr
}

func (writer *closeErrorWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (writer *closeErrorWriter) Close() error {
	writer.closeCalls++
	return writer.closeErr
}

func ExampleNewLog() {
	rand.Seed(11)

	monkey.Patch(time.Now, func() time.Time { return stopped })
	defer monkey.Unpatch(time.Now)

	created := time.Now()
	fmt.Println(NewLog("apache_common", created))
	fmt.Println(NewLog("apache_combined", created))
	fmt.Println(NewLog("apache_error", created))
	fmt.Println(NewLog("rfc3164", created))
	fmt.Println(NewLog("rfc5424", created))
	fmt.Println(NewLog("common_log", created))
	fmt.Println(NewLog("unknown", created))
	fmt.Println(NewLog("json", created))
	// Output:
	// 222.83.191.222 - - [22/Apr/2018:09:30:00 +0000] "DELETE /innovate/next-generation HTTP/1.1" 406 7610
	// 144.199.149.125 - waelchi7603 [22/Apr/2018:09:30:00 +0000] "PUT /revolutionary HTTP/1.1" 301 8089 "https://www.futureaggregate.io/users" "Mozilla/5.0 (Macintosh; PPC Mac OS X 10_6_5 rv:4.0; en-US) AppleWebKit/536.38.2 (KHTML, like Gecko) Version/6.0 Safari/536.38.2"
	// [Sun Apr 22 09:30:00 2018] [eaque:error] [pid 3748:tid 2783] [client 54.26.161.221:31944] Backing up the program won't do anything, we need to compress the optical PCI bandwidth!
	// <94>Apr 22 09:30:00 ortiz5384 vel[1775]: If we copy the firewall, we can get to the PCI firewall through the redundant SQL port!
	// <23>3 2018-04-22T09:30:00.000Z humaniterate.io iusto 544 ID177 - Use the optical RAM hard drive, then you can program the auxiliary feed!
	// 195.44.200.155 - kihn6187 [22/Apr/2018:09:30:00 +0000] "GET /revolutionary/e-markets/holistic/syndicate HTTP/2.0" 404 14503
	//
	// {"host":"13.108.182.26", "user-identifier":"bailey7205", "datetime":"22/Apr/2018:09:30:00 +0000", "method": "GET", "request": "/out-of-the-box/architectures/embrace", "protocol":"HTTP/1.0", "status":200, "bytes":5921, "referer": "http://www.dynamicexperiences.io/robust"}
}

func TestNewSplitFileName(t *testing.T) {
	a := assert.New(t)

	splitFileName := NewSplitFileName("/path/to/file/generated.log", 1)
	a.Equal("/path/to/file/generated1.log", splitFileName, "filename should be '/path/to/file/generated1.log'")
}

func TestGenerateDoesNotCloseWriterTwiceWhenLineSplitCloseFails(t *testing.T) {
	a := assert.New(t)
	closeErr := errors.New("close failed")
	writer := &closeErrorWriter{closeErr: closeErr}

	monkey.Patch(NewWriter, func(string, string, string) (io.WriteCloser, error) {
		return writer, nil
	})
	defer monkey.Unpatch(NewWriter)

	err := Generate(&Option{
		Format:  "apache_common",
		Type:    "log",
		Number:  3,
		SplitBy: 1,
	})

	a.Equal(closeErr, err)
	a.Equal(1, writer.closeCalls)
}

func TestGenerateDoesNotCloseWriterTwiceWhenByteSplitCloseFails(t *testing.T) {
	a := assert.New(t)
	closeErr := errors.New("close failed")
	writer := &closeErrorWriter{closeErr: closeErr}

	monkey.Patch(NewWriter, func(string, string, string) (io.WriteCloser, error) {
		return writer, nil
	})
	defer monkey.Unpatch(NewWriter)

	err := Generate(&Option{
		Format:  "apache_common",
		Type:    "log",
		Bytes:   1,
		SplitBy: 1,
	})

	a.Equal(closeErr, err)
	a.Equal(1, writer.closeCalls)
}

func TestGeneratePreservesWriteErrorWhenCloseFails(t *testing.T) {
	a := assert.New(t)
	writeErr := errors.New("write failed")
	writer := &writeCloseErrorWriter{writeErr: writeErr, closeErr: errors.New("close failed")}

	monkey.Patch(NewWriter, func(string, string, string) (io.WriteCloser, error) {
		return writer, nil
	})
	defer monkey.Unpatch(NewWriter)

	err := Generate(&Option{Format: "apache_common", Type: "log", Number: 1})

	a.Equal(writeErr, err)
	a.Equal(1, writer.closeCalls)
}

func TestGenerateContextDoesNotCloseFileWriterDuringWrite(t *testing.T) {
	a := assert.New(t)
	writer := &blockingWriter{writeStarted: make(chan struct{}), writeRelease: make(chan struct{}), closeCalled: make(chan struct{})}

	monkey.Patch(NewWriter, func(string, string, string) (io.WriteCloser, error) {
		return writer, nil
	})
	defer monkey.Unpatch(NewWriter)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- GenerateContext(ctx, &Option{Format: "apache_common", Type: "log", Number: 1})
	}()

	<-writer.writeStarted
	cancel()
	select {
	case <-writer.closeCalled:
		t.Fatal("file writer was closed while Write was blocked")
	case <-time.After(50 * time.Millisecond):
	case <-done:
		t.Fatal("generation completed before the blocked write was released")
	}
	close(writer.writeRelease)
	a.NoError(<-done)
	select {
	case <-writer.closeCalled:
	case <-time.After(time.Second):
		t.Fatal("file writer was not closed after generation completed")
	}
}
