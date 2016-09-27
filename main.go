package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"

	"github.com/urfave/cli"
	"github.com/gorilla/websocket"
)

func main() {
	app := cli.NewApp()
	app.Name = "wscat"
	app.Usage = "cat, but for websockets"
	app.Action = ActionMain

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "origin",
			Value:  "samehost",
			Usage:  "URL to use for the origin header ('samehost' is special)",
			EnvVar: "WSCAT_ORIGIN",
		},
		cli.StringSliceFlag{
			Name:   "header, H",
			Usage:  "headers to pass to the remote",
			Value:  &cli.StringSlice{},
			EnvVar: "WSCAT_HEADER",
		},
	}

	app.Run(os.Args)
}

var RegexParseHeader = regexp.MustCompile("^\\s*([^\\:]+)\\s*:\\s*(.*)$")

func MustParseHeader(header string) (string, string) {
	if !RegexParseHeader.MatchString(header) {
		log.Fatalf("Unable to parse header: %v (re: %v)", header,
			RegexParseHeader.String())
		return "", ""
	}

	parts := RegexParseHeader.FindStringSubmatch(header)
	return parts[1], parts[2]
}

func MustParseHeaders(c *cli.Context) http.Header {
	headers := http.Header{}

	for _, h := range c.StringSlice("header") {
		key, value := MustParseHeader(h)
		headers.Set(key, value)
	}

	return headers
}

func MustParseURL(u string) *url.URL {
	tgt, err := url.ParseRequestURI(u)
	if err != nil {
		log.Fatalf("Unable to parse URL: %v: %v", u, err)
	}
	switch tgt.Scheme {
	case "http":
		tgt.Scheme = "ws"
	case "https":
		tgt.Scheme = "wss"
	}
	return tgt
}

func ActionMain(c *cli.Context) {

	args := c.Args()

	if len(args) < 1 {
		log.Fatalf("usage: wscat <url>")
	}

	urlString := args.First()

	u := MustParseURL(urlString)

	headers := MustParseHeaders(c)
	origin := c.String("origin")
	if origin == "samehost" {
		origin = "//" + u.Host
	}
	headers.Set("Origin", origin)

	if u.User != nil {
		userPassBytes := []byte(u.User.String() + ":")
		token := base64.StdEncoding.EncodeToString(userPassBytes)
		headers.Set("Authorization", fmt.Sprintf("Basic %v", token))
		u.User = nil
	}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			err = fmt.Errorf("%v: response: %v", err, resp.Status)
		}
		log.Fatalf("Error dialing: %v", err)
	}
	defer conn.Close()

	errc := make(chan error)

	go func() {
		// _, err := io.Copy(os.Stdout, conn)
		var (
			err error
			r   io.Reader
		)
		for {
			_, r, err = conn.NextReader()
			if err != nil {
				break
			}
			_, err = io.Copy(os.Stdout, r)
			if err != nil {
				break
			}
		}
		if err != io.EOF {
			log.Printf("Error copying to stdout: %v", err)
		}
		errc <- err
	}()

	go func() {
		var (
			err error
			w   io.Writer
		)

		for {
			w, err = conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				break
			}
			_, err = io.Copy(w, os.Stdin)
			if err != nil {
				break
			}

			break
		}

		if err != nil && err != io.EOF {
			log.Printf("Error copying from stdin: %v", err)
		}

		errc <- err
	}()

	<-errc
}
