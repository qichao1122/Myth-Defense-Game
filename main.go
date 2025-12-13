package main

import (
	"bytes"
	"embed"
	"image"
	_ "image/png"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/lafriks/go-tiled"
)

// Embed all files inside the asset folder
//
//go:embed asset/*
var assetsFS embed.FS

const (
	MapPath  = "asset/map1.tmx"
	MapPath2 = "asset/map2.tmx"
	MapPath3 = "asset/map3.tmx"
)

var (
	titleMap        *tiled.Map
	titleTileImages map[uint32]*ebiten.Image
	windowWidth     = 800
	windowHeight    = 800
)

type MythDefense struct {
	Enemies []*Enemies
}

type Enemies struct {
	X, Y      float64
	Speed     float64
	Img       *ebiten.Image
	Type      string
	Path      []Node // A* path
	PathIndex int
	Health    int
}

type Node struct {
	X, Y    int
	G, H, F float64
	Parent  *Node
}

func heuristic(a, b Node) float64 {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	return dx*dx + dy*dy // squared Euclidean
}

func AStar(grid [][]int, start, end Node) []Node {
	open := []*Node{&start}
	closed := map[[2]int]bool{}

	for len(open) > 0 {
		current := open[0]
		index := 0
		for i, n := range open {
			if n.F < current.F {
				current = n
				index = i
			}
		}
		open = append(open[:index], open[index+1:]...)
		closed[[2]int{current.X, current.Y}] = true

		if current.X == end.X && current.Y == end.Y {
			var path []Node
			for n := current; n != nil; n = n.Parent {
				path = append([]Node{*n}, path...)
			}
			return path
		}

		directions := [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
		for _, d := range directions {
			neighborX, neighborY := current.X+d[0], current.Y+d[1]
			if neighborX < 0 || neighborY < 0 || neighborY >= len(grid) || neighborX >= len(grid[0]) {
				continue
			}
			if grid[neighborY][neighborX] == 1 || closed[[2]int{neighborX, neighborY}] {
				continue
			}
			neighbor := &Node{X: neighborX, Y: neighborY}
			neighbor.G = current.G + 1
			neighbor.H = heuristic(*neighbor, end)
			neighbor.F = neighbor.G + neighbor.H
			neighbor.Parent = current
			open = append(open, neighbor)
		}
	}
	return nil
}

func createGridFromMap(mapData *tiled.Map) [][]int {
	grid := make([][]int, mapData.Height)
	for i := range grid {
		grid[i] = make([]int, mapData.Width)
	}
	for y := 0; y < mapData.Height; y++ {
		for x := 0; x < mapData.Width; x++ {
			grid[y][x] = 0
		}
	}
	return grid
}

func (g *MythDefense) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}

func drawTitleMap(screen *ebiten.Image) {
	scaleX := 800.0 / float64(titleMap.Width*titleMap.TileWidth)
	scaleY := 800.0 / float64(titleMap.Height*titleMap.TileHeight)
	drawOpts := &ebiten.DrawImageOptions{}

	for y := 0; y < titleMap.Height; y++ {
		for x := 0; x < titleMap.Width; x++ {
			tile := titleMap.Layers[0].Tiles[y*titleMap.Width+x]
			if tile.Tileset == nil {
				continue
			}
			tileID := tile.Tileset.FirstGID + tile.ID
			img := titleTileImages[tileID]
			if img == nil {
				continue
			}
			drawOpts.GeoM.Reset()
			drawOpts.GeoM.Scale(scaleX, scaleY)
			drawOpts.GeoM.Translate(float64(x*titleMap.TileWidth)*scaleX, float64(y*titleMap.TileHeight)*scaleY)
			screen.DrawImage(img, drawOpts)
		}
	}
}

func drawEnemies(screen *ebiten.Image, enemies []*Enemies) {
	for _, enemy := range enemies {
		if enemy.Img == nil {
			continue
		}
		drawOpts := &ebiten.DrawImageOptions{}
		drawOpts.GeoM.Translate(enemy.X, enemy.Y)
		screen.DrawImage(enemy.Img, drawOpts)
	}
}

func UpdateEnemies(enemies []*Enemies, dt float64) {
	for _, e := range enemies {
		if e.Path == nil || e.PathIndex >= len(e.Path) {
			continue
		}

		target := e.Path[e.PathIndex]
		targetX := float64(target.X * 32)
		targetY := float64(target.Y * 32)
		distanceX := targetX - e.X
		distanceY := targetY - e.Y
		distance := math.Hypot(distanceX, distanceY)

		if distance < e.Speed*dt {
			e.X = targetX
			e.Y = targetY
			e.PathIndex++
		} else {
			e.X += distanceX / distance * e.Speed * dt
			e.Y += distanceY / distance * e.Speed * dt
		}
	}
}

func (g *MythDefense) Update() error {
	UpdateEnemies(g.Enemies, 1.0)
	return nil
}

func (g *MythDefense) Draw(screen *ebiten.Image) {
	drawTitleMap(screen)
	drawEnemies(screen, g.Enemies)
}

func loadTiles(mapData *tiled.Map) map[uint32]*ebiten.Image {
	tileImages := make(map[uint32]*ebiten.Image)

	for _, ts := range mapData.Tilesets {
		imgPath := "asset/" + ts.Image.Source
		imgData, err := assetsFS.ReadFile(imgPath)
		if err != nil {
			continue
		}
		img, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(imgData))
		if err != nil {
			continue
		}

		imgBounds := img.Bounds()
		imgWidth := imgBounds.Dx()
		imgHeight := imgBounds.Dy()

		columns := ts.Columns
		if columns == 0 {
			columns = (imgWidth - ts.Margin) / (ts.TileWidth + ts.Spacing)
			if columns <= 0 {
				columns = 1
			}
		}

		tileCount := ts.TileCount
		if tileCount == 0 {
			rows := (imgHeight - ts.Margin) / (ts.TileHeight + ts.Spacing)
			tileCount = columns * rows
		}

		for i := 0; i < tileCount; i++ {
			col := i % columns
			row := i / columns
			tileID := ts.FirstGID + uint32(i)

			x0 := ts.Margin + col*(ts.TileWidth+ts.Spacing)
			y0 := ts.Margin + row*(ts.TileHeight+ts.Spacing)
			x1 := x0 + ts.TileWidth
			y1 := y0 + ts.TileHeight

			if x1 > imgWidth || y1 > imgHeight {
				break
			}

			subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image)
			tileImages[tileID] = subImg
		}
	}
	return tileImages
}

func main() {
	maps := []*tiled.Map{}
	tiles := []map[uint32]*ebiten.Image{}

	for _, path := range []string{MapPath, MapPath2, MapPath3} {
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			continue
		}
		loadedMap, err := tiled.LoadReader(path, bytes.NewReader(data))
		if err != nil {
			continue
		}
		maps = append(maps, loadedMap)
		tiles = append(tiles, loadTiles(loadedMap))
	}

	if len(maps) > 0 {
		titleMap = maps[0]
		titleTileImages = tiles[0]
	}

	enemyPaths := []string{
		"asset/fast_enemy.png",
		"asset/slow_enemy.png",
		"asset/resistant_enemy.png",
		"asset/special_enemy.png",
	}

	enemyImages := []*ebiten.Image{}
	for _, path := range enemyPaths {
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			continue
		}
		loadImage, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		enemyImages = append(enemyImages, loadImage)
	}

	var grid [][]int
	if titleMap != nil {
		grid = createGridFromMap(titleMap)
	} else {
		grid = make([][]int, 25)
		for i := range grid {
			grid[i] = make([]int, 25)
		}
	}

	start := Node{X: 0, Y: 0}
	end := Node{X: 20, Y: 20}
	path := AStar(grid, start, end)
	if path == nil {
		path = []Node{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 20}}
	}

	enemies := []*Enemies{}
	if len(enemyImages) >= 1 {
		enemies = append(enemies, &Enemies{X: 0, Y: 0, Img: enemyImages[0], Type: "fast", Speed: 2.0, Health: 100, Path: path, PathIndex: 0})
	}
	if len(enemyImages) >= 2 {
		enemies = append(enemies, &Enemies{X: 0, Y: 50, Img: enemyImages[1], Type: "slow", Speed: 0.8, Health: 120, Path: path, PathIndex: 0})
	}
	if len(enemyImages) >= 3 {
		enemies = append(enemies, &Enemies{X: 0, Y: 100, Img: enemyImages[2], Type: "resistant", Speed: 1.2, Health: 150, Path: path, PathIndex: 0})
	}
	if len(enemyImages) >= 4 {
		enemies = append(enemies, &Enemies{X: 0, Y: 150, Img: enemyImages[3], Type: "special", Speed: 1.5, Health: 200, Path: path, PathIndex: 0})
	}
	if len(enemyImages) >= 5 {
		enemies = append(enemies, &Enemies{X: 0, Y: 200, Img: enemyImages[4], Type: "basic", Speed: 1.0, Health: 80, Path: path, PathIndex: 0})
	}

	mythGame := &MythDefense{
		Enemies: enemies,
	}

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("Myth Defense - Title Screen")
	ebiten.RunGame(mythGame)
}
