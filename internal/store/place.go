package store

// placeGap is the space FreeSpot leaves between nodes.
const placeGap = 40

// FreeSpot returns a position for a w×h node next to near that overlaps none
// of nodes: to its right first, then below, left and above, ring by ring.
// Drawings and open group frames don't take room (a node may go over them).
func FreeSpot(nodes []Node, near Node, w, h float64) (x, y float64) {
	col := func(i int) float64 {
		switch {
		case i > 0:
			return near.X + near.W + placeGap + float64(i-1)*(w+placeGap)
		case i < 0:
			return near.X - placeGap - w + float64(i+1)*(w+placeGap)
		}
		return near.X
	}
	row := func(j int) float64 {
		switch {
		case j > 0:
			return near.Y + near.H + placeGap + float64(j-1)*(h+placeGap)
		case j < 0:
			return near.Y - placeGap - h + float64(j+1)*(h+placeGap)
		}
		return near.Y
	}
	free := func(x, y float64) bool {
		for _, n := range nodes {
			if n.Kind == KindDrawing || n.Group != nil && !n.Group.Collapsed {
				continue
			}
			if x < n.X+n.W+placeGap/2 && n.X < x+w+placeGap/2 && y < n.Y+n.H+placeGap/2 && n.Y < y+h+placeGap/2 {
				return false
			}
		}
		return true
	}
	// 0, 1, -1, 2, -2… up to r
	around := func(r int) []int {
		out := []int{0}
		for k := 1; k <= r; k++ {
			out = append(out, k, -k)
		}
		return out
	}
	for r := 1; r <= 8; r++ {
		var cells [][2]int
		for _, j := range around(r) { // right column
			cells = append(cells, [2]int{r, j})
		}
		for _, i := range around(r - 1) { // bottom row
			cells = append(cells, [2]int{i, r})
		}
		for _, j := range around(r) { // left column
			cells = append(cells, [2]int{-r, j})
		}
		for _, i := range around(r - 1) { // top row
			cells = append(cells, [2]int{i, -r})
		}
		for _, c := range cells {
			if x, y := col(c[0]), row(c[1]); free(x, y) {
				return x, y
			}
		}
	}
	// A crowded canvas: right of everything.
	right := near.X + near.W
	for _, n := range nodes {
		right = max(right, n.X+n.W)
	}
	return right + placeGap, near.Y
}
