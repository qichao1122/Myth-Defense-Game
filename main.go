package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "os"

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

type MythDefense struct{}

// Layout returns the window size
func (g *MythDefense) Layout(outsideWidth, outsideHeight int) (int, int) {
	return windowWidth, windowHeight
}

// Draw the title map scaled to fit the window
func drawTitleMap(screen *ebiten.Image) {
	scaleX := 800.0 / float64(titleMap.Width*titleMap.TileWidth)
	scaleY := 800.0 / float64(titleMap.Height*titleMap.TileHeight)

	opts := &ebiten.DrawImageOptions{}
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

			opts.GeoM.Reset()
			opts.GeoM.Scale(scaleX, scaleY)
			opts.GeoM.Translate(float64(x*titleMap.TileWidth)*scaleX, float64(y*titleMap.TileHeight)*scaleY)
			screen.DrawImage(img, opts)

		}
	}
}

func (g *MythDefense) Update() error {
	return nil
}

func (g *MythDefense) Draw(screen *ebiten.Image) {
	drawTitleMap(screen)
}

// Load tiles from a Tiled map
func loadTiles(mapData *tiled.Map) map[uint32]*ebiten.Image {
	tileImages := make(map[uint32]*ebiten.Image)

	for _, ts := range mapData.Tilesets {
		imgPath := "asset/" + ts.Image.Source
		imgData, _ := assetsFS.ReadFile(imgPath)
		img, _, _ := ebitenutil.NewImageFromReader(bytes.NewReader(imgData))

		columns := ts.Columns
		if columns == 0 {
			columns = 1
		}

		for i := 0; i < ts.TileCount; i++ {
			col := i % columns
			row := i / columns
			tileID := ts.FirstGID + uint32(i)

			x0 := ts.Margin + col*(ts.TileWidth+ts.Spacing)
			y0 := ts.Margin + row*(ts.TileHeight+ts.Spacing)
			x1 := x0 + ts.TileWidth
			y1 := y0 + ts.TileHeight

			subImg := img.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image)
			tileImages[tileID] = subImg
		}
	}

	return tileImages
}

func main() {
	tmxData1, _ := assetsFS.ReadFile(MapPath)
	titleMap, _ = tiled.LoadReader("map1.tmx", bytes.NewReader(tmxData1))
	titleTileImages = loadTiles(titleMap)

	tmxData2, _ := assetsFS.ReadFile(MapPath)
	titleMap, _ = tiled.LoadReader("map2.tmx", bytes.NewReader(tmxData2))
	titleTileImages = loadTiles(titleMap)

	tmxData3, _ := assetsFS.ReadFile(MapPath)
	titleMap, _ = tiled.LoadReader("map3.tmx", bytes.NewReader(tmxData3))
	titleTileImages = loadTiles(titleMap)

	fmt.Println("Map size in tiles:", titleMap.Width, "x", titleMap.Height)

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("Myth Defense - Title Screen")
	ebiten.RunGame(&MythDefense{})
}
