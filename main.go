package main // import "github.com/tcolgate/frogcam"

import (
	"flag"
	"fmt"
	"image"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func main() {
	dev := flag.String("d", "/dev/video0", "video device to use")
	fmtstr := flag.String("f", "", "video format to use, default first supported")
	szstr := flag.String("s", "", "frame size to use, default largest one")
	addr := flag.String("l", ":8080", "addr to listien")
	mfps := flag.Duration("motion.fps", (1*time.Second)/10, "Duration between motion samples")
	n := flag.Uint("n", 2, "sigmadelta N")
	flag.Parse()

	mc, err := newMotionCam(*dev, *szstr, *fmtstr)
	if err != nil {
		log.Printf("create camera failed, %v", err)
		return
	}

	sd := newSigmaDelta(int(*n), image.Rect(0, 0, int(mc.w), int(mc.h)))
	md := newMotionDetector()

	http.Handle("/sigmadelta/", http.StripPrefix("/sigmadelta/", sd))
	http.Handle("/stream", httpVideo(mc.li))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, page)
	})

	go mc.Run()
	go md.Run(back, sdfi, sd, mc.w, mc.h, f, 20, ms)

	go func() {
		for {
			select {
			case m := <-ms:
				log.Printf("got some motion: %d", len(m))
			}
		}
	}()

	log.Fatal(http.ListenAndServe(*addr, nil))
}
