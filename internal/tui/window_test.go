package tui

import "testing"

// oldClampTop is the window arithmetic for rows of height 1 that windowTop replaces.
func oldClampTop(top, cursor, rows, n int) int {
	if cursor < top {
		top = cursor
	}
	if cursor >= top+rows {
		top = cursor - rows + 1
	}
	return max(min(top, n-rows), 0)
}

func TestWindowTop_UniformRowsMatchThePlainWindow(t *testing.T) {
	for n := 1; n <= 8; n++ {
		heights := make([]int, n)
		for i := range heights {
			heights[i] = 1
		}
		for rows := 1; rows <= 9; rows++ {
			for top := 0; top < n; top++ {
				for cursor := 0; cursor < n; cursor++ {
					if got, want := windowTop(heights, top, cursor, rows), oldClampTop(top, cursor, rows, n); got != want {
						t.Fatalf("n=%d rows=%d top=%d cursor=%d: got %d, want %d", n, rows, top, cursor, got, want)
					}
				}
			}
		}
	}
}

func TestWindowTop_TallRowsScrollByLines(t *testing.T) {
	heights := []int{1, 2, 1, 2, 1}
	for _, tc := range []struct{ top, cursor, rows, want int }{
		{0, 3, 4, 2}, // rows 2..3 are 1+2 lines; row 1 would make 5
		{2, 4, 4, 2}, // 1+2+1 fits exactly
		{3, 1, 4, 1}, // moving up pulls the window up
		{0, 0, 4, 0},
		{0, 4, 9, 0}, // everything fits: no scrolling
		{4, 4, 5, 2}, // no blank space left below the last row: 2+1+2+1 = 6 > 5, so 2..4
	} {
		if got := windowTop(heights, tc.top, tc.cursor, tc.rows); got != tc.want {
			t.Errorf("windowTop(top=%d cursor=%d rows=%d) = %d, want %d", tc.top, tc.cursor, tc.rows, got, tc.want)
		}
	}
}

func TestWindowTop_ARowTallerThanTheWindowStillShowsItsFirstLine(t *testing.T) {
	if got := windowTop([]int{1, 2, 1}, 0, 1, 1); got != 1 {
		t.Errorf("got %d, want the tall row at the top", got)
	}
}
