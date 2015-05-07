package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"

	"github.com/codegangsta/cli"
	"golang.org/x/net/websocket"
)

func main() {
	app := cli.NewApp()
	app.Name = "wscat"
	app.Usage = "cat, but for websockets"
	app.Action = ActionMain

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "origin",
			Value:  "http://localhost/",
			Usage:  "value to use for the origin header",
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
	return tgt
}

func ActionMain(c *cli.Context) {

	args := c.Args()

	if len(args) < 1 {
		log.Fatalf("usage: wscat <url>")
	}

	url := args.First()

	config := &websocket.Config{}
	config.Location = MustParseURL(url)
	config.Origin = MustParseURL(c.String("origin"))
	config.Header = MustParseHeaders(c)
	config.Version = websocket.ProtocolVersionHybi13

	conn, err := websocket.DialConfig(config)
	if err != nil {
		log.Fatalf("Error dialing: %v", err)
	}
	defer conn.Close()

	errc := make(chan error)

	go func() {
		_, err := io.Copy(os.Stdout, conn)
		if err != io.EOF && err != nil {
			log.Printf("Error copying to stdout: %v", err)
		}
		errc <- err
	}()

	go func() {
		_, err = io.Copy(conn, os.Stdin)
		if err != io.EOF && err != nil {
			log.Printf("Error copying from stdin: %v", err)
		}
		errc <- err
	}()

	<-errc
}
