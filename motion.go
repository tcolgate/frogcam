package main // import "github.com/tcolgate/frogcam"

import (
	"image"
	"log"
	_ "net/http/pprof"

	"github.com/blackjack/webcam"
	"github.com/disintegration/gift"
	"github.com/harrydb/go/img/grayscale"
)

type motion []grayscale.CoCo

func detectmotion(back chan struct{}, fi chan []byte, sd *sigmadelta, w, h uint32, format webcam.PixelFormat, minCoCo int, ms chan motion) {
	var frame []byte
	var err error
	var img *image.YCbCr
	for {
		bframe := <-fi
		// copy frame
		if len(frame) < len(bframe) {
			frame = make([]byte, len(bframe))
		}
		copy(frame, bframe)
		back <- struct{}{}
		img, frame, err = frameToImage(frame, w, h, format)
		if err != nil {
			log.Printf("err: %v", err)
			continue
		}
		sd.Update(img)

		/*
			dstImage := imaging.Blur(sd.e, 5)
		*/
		g := gift.New(
			gift.GaussianBlur(5),
		)
		dst := image.NewGray(g.Bounds(sd.e.Bounds()))
		g.Draw(dst, sd.e)

		cocos := grayscale.CoCos(dst, 255, grayscale.NEIGHBOR8)
		filteredCoCos := cocos[:0]

		for i := range cocos {
			if len(cocos[i]) > minCoCo {
				filteredCoCos = append(filteredCoCos, cocos[i])
			}
		}

		if len(filteredCoCos) > 0 {
			ms <- motion(filteredCoCos)
		}

		/*
			pal := colorful.FastWarmPalette(len(filteredCoCos))
			for i := range filteredCoCos {
				log.Printf("filteredCoCos[%d]: %d points, %v", i, len(filteredCoCos[i]), pal[i])
			}
		*/
	}
}
