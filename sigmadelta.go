package main

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"strings"
	"sync"
)

type sigmadelta struct {
	sync.RWMutex
	n      int
	bounds image.Rectangle

	m *image.Gray
	o *image.Gray
	v *image.Gray
	e *image.Gray
}

func newSigmaDelta(n int, bounds image.Rectangle) *sigmadelta {
	return &sigmadelta{
		n:      n,
		bounds: bounds,

		m: image.NewGray(bounds),
		o: image.NewGray(bounds),
		v: image.NewGray(bounds),
		e: image.NewGray(bounds),
	}
}

func (s *sigmadelta) Update(in *image.YCbCr) error {
	s.Lock()
	defer s.Unlock()

	// mt estimator
	for i := 0; i < len(in.Y); i++ {
		inx := in.Y[i]
		mx := s.m.Pix[i]
		switch {
		case mx < inx:
			s.m.Pix[i] = mx + 1
		case mx > inx:
			s.m.Pix[i] = mx - 1
		default:
		}
	}

	// ot computation
	for i := 0; i < len(in.Y); i++ {
		inx := in.Y[i]
		mx := s.m.Pix[i]

		oy := int(mx) - int(inx)
		if oy <= 0 {
			oy *= -1
		}

		s.o.Pix[i] = uint8(oy)
	}

	// vt update
	for i := 0; i < len(in.Y); i++ {
		ox := s.o.Pix[i]
		vx := s.v.Pix[i]
		intvx := int(vx)

		not := s.n * int(ox)
		switch {
		case intvx < not:
			if intvx < 255 {
				s.v.Pix[i] = vx + 1
			}
		case intvx > not:
			if intvx > 1 {
				s.v.Pix[i] = vx - 1
			}
		default:
		}
	}

	// et estimate
	for i := 0; i < len(in.Y); i++ {
		ox := s.o.Pix[i]
		vx := s.v.Pix[i]

		switch {
		case ox < vx:
			s.e.Pix[i] = 0
		default:
			s.e.Pix[i] = 255
		}
	}

	return nil
}

func (s *sigmadelta) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.RLock()
	defer s.RUnlock()

	parts := strings.Split(r.URL.Path, "/")
	switch parts[0] {
	case "":
		fmt.Fprintf(w, "<html><body>Hello</body></html>")
	case "m":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.m)
	case "o":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.o)
	case "v":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.v)
	case "e":
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, s.e)
	default:
		http.Error(w, "file not found", 404)
	}
}
