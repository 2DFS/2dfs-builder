package filesystem

import (
	"fmt"
	"testing"
)

func checkRows(n int, f Field, t *testing.T) {
	//access all rows
	for i := 0; i <= n; i++ {
		_ = f.(*TwoDFilesystem).Rows[i]
	}

	//Check TotRows to be n+1
	if f.(*TwoDFilesystem).TotRows != n+1 {
		t.Fatalf("f.TotRows should be n+1, expected %d actual %d", n, f.(*TwoDFilesystem).TotRows)
	}

	//check rows length against TotRows
	actualRowsSize := len(f.(*TwoDFilesystem).Rows)

	if f.(*TwoDFilesystem).TotRows != actualRowsSize {
		t.Fatalf("Rows size should match f.TotRows, expected %d actual %d", f.(*TwoDFilesystem).TotRows, actualRowsSize)
	}
}

func checkAllotments(row int, n int, f Field, t *testing.T) {
	//access all rows
	for i := 0; i <= row; i++ {
		_ = f.(*TwoDFilesystem).Rows[i]
	}

	//Check TotAllotments to be n+1
	if f.(*TwoDFilesystem).Rows[row].TotAllotments != n+1 {
		t.Fatalf("f.TotAllotments should be n+1, expected %d actual %d", n, f.(*TwoDFilesystem).TotRows)
	}

	//check rows length against TotRows
	actualAllotmentsSize := len(f.(*TwoDFilesystem).Rows[row].Allotments)

	if f.(*TwoDFilesystem).Rows[row].TotAllotments != actualAllotmentsSize {
		t.Fatalf("Allotments size should match f.Row[row].TotAllotments, expected %d actual %d", f.(*TwoDFilesystem).TotRows, actualAllotmentsSize)
	}
}

func isTheSame(f1 Field, f2 Field, t *testing.T) {
	if f1.(*TwoDFilesystem).TotRows != f2.(*TwoDFilesystem).TotRows {
		t.Fatalf("two different rows values in received filesystems")
	}
	for i, r := range f1.(*TwoDFilesystem).Rows {
		if r.TotAllotments != f2.(*TwoDFilesystem).Rows[i].TotAllotments {
			t.Fatalf("two different allotment values in received filesystems")
		}
		for j, a1 := range r.Allotments {
			a2 := f2.(*TwoDFilesystem).Rows[i].Allotments[j]
			if a1.Row != a2.Row || a1.Col != a2.Col || a1.Digest != a2.Digest {
				t.Fatalf("two different allotments expected %v, actual %v", a1, a2)
			}
		}
	}
}

func TestAdd10Rows(t *testing.T) {
	n := 10

	//empty field
	f := GetField()

	//gen rows from 0 to 10
	f.(*TwoDFilesystem).genRows(n)

	//check generated rows
	checkRows(n, f, t)
}

func TestAdd1Row(t *testing.T) {
	n := 0

	//empty field
	f := GetField()

	//gen rows from 0 to 10
	f.(*TwoDFilesystem).genRows(n)

	//check generated rows
	checkRows(n, f, t)
}

func TestAdd0Rows(t *testing.T) {
	n := 10

	//empty field
	f := GetField()

	//gen rows from 0 to 10
	f.(*TwoDFilesystem).genRows(n)

	//check generated rows
	checkRows(n, f, t)

	//gen rows 0 to 5
	f.(*TwoDFilesystem).genRows(5)

	//check generated rows
	checkRows(n, f, t)
}

func TestAdd10Allotments(t *testing.T) {
	n := 10
	row := 10

	//empty Field
	f := GetField()

	//gen allotments 10:[0-9]
	f.(*TwoDFilesystem).genAllotments(row, n)

	//check generate allotments
	checkAllotments(row, n, f, t)
}

func TestAdd1Allotment(t *testing.T) {
	n := 0
	row := 0

	//empty Field
	f := GetField()

	//gen allotments 10:[0-9]
	f.(*TwoDFilesystem).genAllotments(row, n)

	//check generate allotments
	checkAllotments(row, n, f, t)
}

func TestAdd0Allotment(t *testing.T) {
	n := 10
	row := 10

	//empty Field
	f := GetField()

	//gen allotments 10:[0-9]
	f.(*TwoDFilesystem).genAllotments(row, n)

	//check generate allotments
	checkAllotments(row, n, f, t)

	//gen allotments 10:[0-9]
	f.(*TwoDFilesystem).genAllotments(row, n-3)

	//check generate allotments
	checkAllotments(row, n, f, t)
}

func TestAddField(t *testing.T) {
	n := 3
	row := 0

	//empty Field
	f := GetField()

	for r := 0; r <= row; r++ {
		for i := 0; i <= n; i++ {

			//add allotments
			f.AddAllotment(Allotment{
				Row:    r,
				Col:    i,
				Digest: fmt.Sprintf("r%d-c%d", r, i),
			})

		}
	}

	//check allotments values
	for r := 0; r <= row; r++ {
		for i := 0; i <= n; i++ {

			expecteddigest := fmt.Sprintf("r%d-c%d", r, i)
			actual := f.(*TwoDFilesystem).Rows[r].Allotments[i]
			if expecteddigest != actual.Digest || r != actual.Row || i != actual.Col {
				t.Fatalf("expected allotment digest %s, actual allotment %v", expecteddigest, actual)
			}

		}
	}
	checkAllotments(row, n, f, t)

}

func TestMarshalUnmarshal(t *testing.T) {
	n := 3
	row := 0

	//empty Field
	f := GetField()

	for r := 0; r <= row; r++ {
		for i := 0; i <= n; i++ {

			//add allotments
			f.AddAllotment(Allotment{
				Row:    r,
				Col:    i,
				Digest: fmt.Sprintf("r%d-c%d", r, i),
			})

		}
	}

	//Marshal
	marshaled := f.Marshal()
	unmarshaled, err := f.Unmarshal(marshaled)

	if err != nil {
		t.Fatalf("%v", err)
	}

	isTheSame(f, unmarshaled, t)
}
