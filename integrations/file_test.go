// Copyright 2020 The goftp Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package integrations

import (
	"io/ioutil"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goftp.io/server/v2"
	"goftp.io/server/v2/driver/file"

	"github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/assert"
)

func TestFileDriver(t *testing.T) {
	err := os.MkdirAll("./testdata", os.ModePerm)
	assert.NoError(t, err)

	var perm = server.NewSimplePerm("test", "test")
	driver, err := file.NewDriver("./testdata")
	assert.NoError(t, err)

	opt := &server.Options{
		Name:   "test ftpd",
		Driver: driver,
		Perm:   perm,
		Port:   2122,
		Auth: &server.SimpleAuth{
			Name:     "admin",
			Password: "admin",
		},
		Logger: new(server.DiscardLogger),
	}

	runServer(t, opt, nil, func() {
		// Give server 0.5 seconds to get to the listening state
		timeout := time.NewTimer(time.Millisecond * 500)

		for {
			f, err := ftp.Connect("localhost:2122")
			if err != nil && len(timeout.C) == 0 { // Retry errors
				continue
			}
			assert.NoError(t, err)

			assert.NoError(t, f.Login("admin", "admin"))
			assert.Error(t, f.Login("admin", ""))

			var content = `test`
			assert.NoError(t, f.Stor("server_test.go", strings.NewReader(content)))

			names, err := f.NameList("/")
			assert.NoError(t, err)
			assert.EqualValues(t, 1, len(names))
			assert.EqualValues(t, "server_test.go", names[0])

			bs, err := ioutil.ReadFile("./testdata/server_test.go")
			assert.NoError(t, err)
			assert.EqualValues(t, content, string(bs))

			entries, err := f.List("/")
			assert.NoError(t, err)
			assert.EqualValues(t, 1, len(entries))
			assert.EqualValues(t, "server_test.go", entries[0].Name)
			assert.EqualValues(t, 4, entries[0].Size)
			assert.EqualValues(t, ftp.EntryTypeFile, entries[0].Type)

			curDir, err := f.CurrentDir()
			assert.NoError(t, err)
			assert.EqualValues(t, "/", curDir)

			size, err := f.FileSize("/server_test.go")
			assert.NoError(t, err)
			assert.EqualValues(t, 4, size)

			r, err := f.RetrFrom("/server_test.go", 2)
			assert.NoError(t, err)

			buf, err := ioutil.ReadAll(r)
			r.Close()
			assert.NoError(t, err)
			assert.EqualValues(t, "st", string(buf))

			err = f.Rename("/server_test.go", "/test.go")
			assert.NoError(t, err)

			err = f.MakeDir("/src")
			assert.NoError(t, err)

			err = f.Delete("/test.go")
			assert.NoError(t, err)

			err = f.ChangeDir("/src")
			assert.NoError(t, err)

			curDir, err = f.CurrentDir()
			assert.NoError(t, err)
			assert.EqualValues(t, "/src", curDir)

			assert.NoError(t, f.Stor("server_test.go", strings.NewReader(content)))

			r, err = f.Retr("/src/server_test.go")
			assert.NoError(t, err)

			buf, err = ioutil.ReadAll(r)
			r.Close()
			assert.NoError(t, err)
			assert.EqualValues(t, "test", string(buf))

			err = f.RemoveDir("/src")
			assert.NoError(t, err)

			curDir, err = f.CurrentDir()
			assert.NoError(t, err)
			assert.EqualValues(t, "/", curDir)

			assert.NoError(t, f.Stor(" file_name .test", strings.NewReader("tttt")))
			assert.NoError(t, f.Delete(" file_name .test"))

			err = f.Quit()
			assert.NoError(t, err)

			break
		}
	})
}

func TestMLST(t *testing.T) {
	err := os.MkdirAll("./testdata", os.ModePerm)
	assert.NoError(t, err)

	var perm = server.NewSimplePerm("test", "test")
	driver, err := file.NewDriver("./testdata")
	assert.NoError(t, err)

	opt := &server.Options{
		Name:   "test ftpd",
		Driver: driver,
		Perm:   perm,
		Port:   2124,
		Auth: &server.SimpleAuth{
			Name:     "admin",
			Password: "admin",
		},
		Logger: new(server.DiscardLogger),
	}

	runServer(t, opt, nil, func() {
		// Give server 0.5 seconds to get to the listening state
		timeout := time.NewTimer(time.Millisecond * 500)

		for {
			f, err := ftp.Connect("localhost:2124")
			if err != nil && len(timeout.C) == 0 { // Retry errors
				continue
			}
			assert.NoError(t, err)

			assert.NoError(t, f.Login("admin", "admin"))

			// Check that MLST is supported (implies MLSD too)
			assert.True(t, f.IsTimePreciseInList(), "MLST should be advertised in FEAT")

			// Store a test file
			var content = `hello`
			assert.NoError(t, f.Stor("mlst_test.txt", strings.NewReader(content)))

			// Test MLST on a file
			entry, err := f.GetEntry("/mlst_test.txt")
			assert.NoError(t, err)
			assert.EqualValues(t, "mlst_test.txt", entry.Name)
			assert.EqualValues(t, 5, entry.Size)
			assert.EqualValues(t, ftp.EntryTypeFile, entry.Type)

			// Test MLST on a directory
			assert.NoError(t, f.MakeDir("/mlst_dir"))
			entry, err = f.GetEntry("/mlst_dir")
			assert.NoError(t, err)
			assert.EqualValues(t, "mlst_dir", entry.Name)
			assert.EqualValues(t, ftp.EntryTypeFolder, entry.Type)

			// Test MLST on current directory (no param)
			entry, err = f.GetEntry("")
			assert.NoError(t, err)
			assert.EqualValues(t, ftp.EntryTypeFolder, entry.Type)

			// Test that List uses MLSD (machine-readable format)
			entries, err := f.List("/")
			assert.NoError(t, err)
			found := false
			for _, e := range entries {
				if e.Name == "mlst_test.txt" {
					found = true
					assert.EqualValues(t, 5, e.Size)
					assert.EqualValues(t, ftp.EntryTypeFile, e.Type)
				}
			}
			assert.True(t, found, "mlst_test.txt should be in listing")

			// Cleanup
			assert.NoError(t, f.Delete("/mlst_test.txt"))
			assert.NoError(t, f.RemoveDir("/mlst_dir"))

			err = f.Quit()
			assert.NoError(t, err)

			break
		}
	})
}

func TestSTAT(t *testing.T) {
	err := os.MkdirAll("./testdata", os.ModePerm)
	assert.NoError(t, err)

	filePath := filepath.Join("./testdata", "stat_test.txt")
	err = os.WriteFile(filePath, []byte("stat"), 0600)
	assert.NoError(t, err)

	var perm = server.NewSimplePerm("test", "test")
	driver, err := file.NewDriver("./testdata")
	assert.NoError(t, err)

	opt := &server.Options{
		Name:   "test ftpd",
		Driver: driver,
		Perm:   perm,
		Port:   2126,
		Auth: &server.SimpleAuth{
			Name:     "admin",
			Password: "admin",
		},
		Logger: new(server.DiscardLogger),
	}

	runServer(t, opt, nil, func() {
		// Give server 0.5 seconds to get to the listening state
		timeout := time.NewTimer(time.Millisecond * 500)

		for {
			conn, err := net.Dial("tcp", "localhost:2126")
			if err != nil && len(timeout.C) == 0 { // Retry errors
				continue
			}
			assert.NoError(t, err)

			protoConn := textproto.NewConn(conn)
			defer protoConn.Close()

			_, _, err = protoConn.ReadResponse(220)
			assert.NoError(t, err)

			send := func(expected int, format string, args ...interface{}) (string, error) {
				_, err := protoConn.Cmd(format, args...)
				if err != nil {
					return "", err
				}
				_, msg, err := protoConn.ReadResponse(expected)
				return msg, err
			}

			_, err = send(331, "USER admin")
			assert.NoError(t, err)
			_, err = send(230, "PASS admin")
			assert.NoError(t, err)

			msg, err := send(211, "STAT")
			assert.NoError(t, err)
			assert.Contains(t, msg, "FTP server status")

			msg, err = send(213, "STAT stat_test.txt")
			assert.NoError(t, err)
			assert.Contains(t, msg, "Status of /stat_test.txt")
			assert.Contains(t, msg, "stat_test.txt")

			_, err = send(221, "QUIT")
			assert.NoError(t, err)

			break
		}
	})
}

func TestLogin(t *testing.T) {
	err := os.MkdirAll("./testdata", os.ModePerm)
	assert.NoError(t, err)

	var perm = server.NewSimplePerm("test", "test")
	driver, err := file.NewDriver("./testdata")
	assert.NoError(t, err)

	// Server options without hostname or port
	opt := &server.Options{
		Name:   "test ftpd",
		Driver: driver,
		Auth: &server.SimpleAuth{
			Name:     "admin",
			Password: "admin",
		},
		Perm:   perm,
		Logger: new(server.DiscardLogger),
	}

	// Start the listener
	l, err := net.Listen("tcp", ":2123")
	assert.NoError(t, err)

	// Start the server using the listener
	s, err := server.NewServer(opt)
	assert.NoError(t, err)
	go func() {
		err := s.Serve(l)
		assert.EqualError(t, err, server.ErrServerClosed.Error())
	}()

	// Give server 0.5 seconds to get to the listening state
	timeout := time.NewTimer(time.Millisecond * 500)
	for {
		f, err := ftp.Connect("localhost:2123")
		if err != nil && len(timeout.C) == 0 { // Retry errors
			continue
		}
		assert.NoError(t, err)

		assert.NoError(t, f.Login("admin", "admin"))
		assert.Error(t, f.Login("admin", ""))

		err = f.Quit()
		assert.NoError(t, err)
		break
	}

	assert.NoError(t, s.Shutdown())
}
