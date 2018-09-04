package main // import "github.com/tcolgate/frogcam"

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/blackjack/webcam"
)

func main() {
	dev := flag.String("d", "/dev/video0", "video device to use")
	fmtstr := flag.String("f", "", "video format to use, default first supported")
	szstr := flag.String("s", "", "frame size to use, default largest one")
	addr := flag.String("l", ":8080", "addr to listien")
	n := flag.Uint("n", 2, "sigmadelta N")
	flag.Parse()

	cam, err := webcam.Open(*dev)
	if err != nil {
		panic(err.Error())
	}
	defer cam.Close()

	// select pixel format
	formatDesc := cam.GetSupportedFormats()

	fmt.Println("Available formats:")
	for f, s := range formatDesc {
		fmt.Fprintf(os.Stderr, "%s (%#x)\n", s, f)
	}

	var format webcam.PixelFormat
FMT:
	for f, s := range formatDesc {
		if *fmtstr == "" {
			if supportedFormats[f] {
				format = f
				break FMT
			}

		} else if *fmtstr == s {
			if !supportedFormats[f] {
				log.Println(formatDesc[f], "format is not supported, exiting")
				return
			}
			format = f
			break
		}
	}
	if format == 0 {
		log.Println("No format found, exiting")
		return
	}

	// select frame size
	frames := byArea(cam.GetSupportedFrameSizes(format))
	sort.Sort(frames)

	fmt.Fprintln(os.Stderr, "Supported frame sizes for format", formatDesc[format])
	for _, f := range frames {
		fmt.Fprintln(os.Stderr, f.GetString())
	}
	var size *webcam.FrameSize
	switch {
	case *szstr == "":
		size = &frames[len(frames)-1]
	case strings.Count(*szstr, "x") == 1:
		parts := strings.Split(*szstr, "x")
		x, xerr := strconv.Atoi(parts[0])
		y, yerr := strconv.Atoi(parts[1])
		if xerr != nil || yerr != nil {
			log.Fatalf("couldn't parse width x height")
		}
		size = &webcam.FrameSize{
			MaxWidth:  uint32(x),
			MaxHeight: uint32(y),
		}
	default:
		for _, f := range frames {
			if *szstr == f.GetString() {
				size = &f
			}
		}
	}

	if size == nil {
		log.Println("No matching frame size, exiting")
		return
	}

	fmt.Fprintln(os.Stderr, "Requesting", formatDesc[format], size.GetString())
	f, w, h, err := cam.SetImageFormat(format, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		log.Println("SetImageFormat return error", err)
		return

	}
	fmt.Fprintf(os.Stderr, "Resulting image format: %s %dx%d\n", formatDesc[f], w, h)

	// start streaming
	err = cam.StartStreaming()
	if err != nil {
		log.Println(err)
		return
	}

	var (
		li   = make(chan *bytes.Buffer)
		fi   = make(chan []byte)
		sdfi = make(chan []byte)
		back = make(chan struct{})
	)

	go encodeToJPEG(back, fi, li, w, h, f)

	sd := newSigmaDelta(int(*n), image.Rect(0, 0, int(w), int(h)))
	go detectmotion(back, sdfi, sd, w, h, f, 20)

	http.Handle("/sigmadelta/", http.StripPrefix("/sigmadelta/", sd))
	http.Handle("/stream", httpVideo(li))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, page)
	})

	timeout := uint32(15) //5 seconds

	go func() {
		for {
			err = cam.WaitForFrame(timeout)
			if err != nil {
				log.Println(err)
				return
			}

			switch err.(type) {
			case nil:
			case *webcam.Timeout:
				log.Println(err)
				continue
			default:
				log.Println(err)
				return
			}

			frame, err := cam.ReadFrame()
			if err != nil {
				log.Println(err)
				return
			}
			if len(frame) != 0 {
				select {
				case fi <- frame:
					<-back
				default:
				}

				select {
				case sdfi <- frame:
					<-back
				default:
				}
			}
		}
	}()

	log.Fatal(http.ListenAndServe(*addr, nil))
}
