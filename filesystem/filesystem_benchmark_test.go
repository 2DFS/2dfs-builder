package filesystem

import (
	"fmt"
	"testing"
	"time"
)

func Test10MAllotments(t *testing.T) {

	init := time.Now().UnixMilli()
	//empty field
	f := GetField()

	//gen 10M allotments
	f.AddAllotment(Allotment{
		Row:    10000,
		Col:    1000,
		Digest: "test",
	})
	for i := 0; i < 9999; i++ {
		for j := 0; j < 999; j++ {
			f.AddAllotment(Allotment{
				Row:    i,
				Col:    j,
				Digest: "test",
			})
		}
	}
	end := time.Now().UnixMilli()

	fmt.Printf("Completed. Size: %d bytes, took: %d ms \n", len(f.Marshal()), end-init)
}
