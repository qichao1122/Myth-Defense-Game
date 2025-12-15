package main

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	_ "image/jpeg"
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
	Enemies          []*Enemies
	Towers           []*Towers
	TowerImages      []*ebiten.Image // Available tower types
	SelectedTower    int             // Index of selected tower (-1 = none)
	DraggingTower    bool            // Whether currently dragging a tower
	DragX, DragY     int             // Current drag position
	PrevMousePressed bool            // Previous frame mouse state
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

type Towers struct {
	X, Y        float64       // Position
	Img         *ebiten.Image // Tower sprite
	TowerHealth int           // Health of the tower
	Range       float64       // Attack range in pixels
	Damage      int           // Damage per attack
	AttackSpeed float64       // Attacks per second
	Cooldown    float64       // Time until next attack
	Target      *Enemies      // Current enemy target
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

func drawTowers(screen *ebiten.Image, towers []*Towers) {
	for _, tower := range towers {
		if tower.Img != nil {
			drawOpts := &ebiten.DrawImageOptions{}
			drawOpts.GeoM.Scale(0.5, 0.5)             // Scale FIRST
			drawOpts.GeoM.Translate(tower.X, tower.Y) // Then translate
			screen.DrawImage(tower.Img, drawOpts)
		}
	}
}

// drawTowerSelectionUI draws the tower selection panel in the bottom-right
func drawTowerSelectionUI(screen *ebiten.Image, game *MythDefense) {
	if len(game.TowerImages) == 0 {
		return
	}

	// UI settings
	iconSize := 60
	padding := 10
	startX := windowWidth - (iconSize + padding)
	startY := windowHeight - (len(game.TowerImages)*(iconSize+padding) + padding)

	// Draw background panel
	for i := 0; i < len(game.TowerImages); i++ {
		y := startY + i*(iconSize+padding)

		// Draw selection highlight (yellow background)
		if i == game.SelectedTower {
			ebitenutil.DrawRect(screen, float64(startX-5), float64(y-5),
				float64(iconSize+10), float64(iconSize+10),
				color.RGBA{255, 255, 0, 128})
		}

		// Draw tower icon
		if game.TowerImages[i] != nil {
			drawOpts := &ebiten.DrawImageOptions{}

			// Scale to fit icon size
			imgBounds := game.TowerImages[i].Bounds()
			scaleX := float64(iconSize) / float64(imgBounds.Dx())
			scaleY := float64(iconSize) / float64(imgBounds.Dy())
			scale := scaleX
			if scaleY < scaleX {
				scale = scaleY
			}

			drawOpts.GeoM.Scale(scale, scale)
			drawOpts.GeoM.Translate(float64(startX), float64(y))
			screen.DrawImage(game.TowerImages[i], drawOpts)
		}

		// Draw white border around icon
		borderThickness := 2.0
		borderColor := color.RGBA{255, 255, 255, 255}
		// Top
		ebitenutil.DrawRect(screen, float64(startX), float64(y),
			float64(iconSize), borderThickness, borderColor)
		// Bottom
		ebitenutil.DrawRect(screen, float64(startX), float64(y+iconSize)-borderThickness,
			float64(iconSize), borderThickness, borderColor)
		// Left
		ebitenutil.DrawRect(screen, float64(startX), float64(y),
			borderThickness, float64(iconSize), borderColor)
		// Right
		ebitenutil.DrawRect(screen, float64(startX+iconSize)-borderThickness, float64(y),
			borderThickness, float64(iconSize), borderColor)
	}
}

// handleTowerSelection checks if mouse click is on tower selection UI
func (g *MythDefense) handleTowerSelection(x, y int) bool {
	iconSize := 60
	padding := 10
	startX := windowWidth - (iconSize + padding)
	startY := windowHeight - (len(g.TowerImages)*(iconSize+padding) + padding)

	for i := 0; i < len(g.TowerImages); i++ {
		iconY := startY + i*(iconSize+padding)

		if x >= startX && x <= startX+iconSize && y >= iconY && y <= iconY+iconSize {
			g.SelectedTower = i
			return true
		}
	}
	return false
}

// placeTower places a tower at the specified position
func (g *MythDefense) placeTower(x, y float64) {
	if g.SelectedTower < 0 || g.SelectedTower >= len(g.TowerImages) {
		return
	}

	// Center the tower image at the cursor position
	imgBounds := g.TowerImages[g.SelectedTower].Bounds()
	scale := 0.5
	offsetX := float64(imgBounds.Dx()) * scale / 2
	offsetY := float64(imgBounds.Dy()) * scale / 2

	// Adjust position to center the tower at cursor
	placementX := x - offsetX
	placementY := y - offsetY

	// Create tower with stats based on type
	var tower *Towers
	switch g.SelectedTower {
	case 0: // Basic tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[0], TowerHealth: 100, Range: 100, Damage: 10, AttackSpeed: 1.0}
	case 1: // Fast tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[1], TowerHealth: 80, Range: 120, Damage: 15, AttackSpeed: 1.5}
	case 2: // Slow tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[2], TowerHealth: 80, Range: 90, Damage: 8, AttackSpeed: 0.5}
	case 3: // Heavy tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[3], TowerHealth: 150, Range: 80, Damage: 25, AttackSpeed: 0.8}
	case 4: // Heavy tower 2
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[4], TowerHealth: 150, Range: 80, Damage: 30, AttackSpeed: 0.7}
	}

	if tower != nil {
		g.Towers = append(g.Towers, tower)
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

func UpdateTowers(towers []*Towers, enemies []*Enemies, frame_time float64) {
	for _, tower := range towers {
		if tower.Cooldown > 0 {
			tower.Cooldown -= frame_time
			continue

		}
		var closest *Enemies
		minDistance := tower.Range
		for _, e := range enemies {
			dx := e.X - tower.X
			dy := e.Y - tower.Y
			distance := math.Hypot(dx, dy)
			if distance <= minDistance {
				closest = e
				minDistance = distance
			}
		}
		if closest != nil {
			closest.Health -= tower.Damage
			tower.Cooldown = 1.0 / tower.AttackSpeed
		}
	}
}

func (g *MythDefense) Update() error {
	deltaTime := 1.0 // or calculate actual elapsed time per frame
	UpdateEnemies(g.Enemies, deltaTime)
	UpdateTowers(g.Towers, g.Enemies, deltaTime)

	// Get current mouse state
	x, y := ebiten.CursorPosition()
	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	// Update drag position every frame if dragging
	if g.DraggingTower {
		g.DragX = x
		g.DragY = y
	}

	// Mouse button just pressed (transition from not pressed to pressed)
	if mousePressed && !g.PrevMousePressed {
		// Check if clicking on tower selection UI to start drag
		if g.handleTowerSelection(x, y) {
			g.DraggingTower = true
			g.DragX = x
			g.DragY = y
		}
	} else if !mousePressed && g.PrevMousePressed {
		// Mouse button just released
		if g.DraggingTower && g.SelectedTower >= 0 {
			// Place tower at the EXACT drag position
			g.placeTower(float64(g.DragX), float64(g.DragY))
			g.DraggingTower = false
			g.SelectedTower = -1 // Deselect after placing
		}
	}

	g.PrevMousePressed = mousePressed

	return nil
}

func (g *MythDefense) Draw(screen *ebiten.Image) {
	drawTitleMap(screen)
	drawEnemies(screen, g.Enemies)
	drawTowers(screen, g.Towers)
	drawTowerSelectionUI(screen, g)

	// Draw dragging tower preview
	if g.DraggingTower && g.SelectedTower >= 0 && g.SelectedTower < len(g.TowerImages) {
		drawOpts := &ebiten.DrawImageOptions{}

		// Make it semi-transparent
		drawOpts.ColorScale.Scale(1, 1, 1, 0.6)

		// Center the image on cursor - MUST match placeTower offset
		imgBounds := g.TowerImages[g.SelectedTower].Bounds()
		scale := 0.5
		offsetX := float64(imgBounds.Dx()) * scale / 2
		offsetY := float64(imgBounds.Dy()) * scale / 2

		drawOpts.GeoM.Scale(scale, scale)
		drawOpts.GeoM.Translate(float64(g.DragX)-offsetX, float64(g.DragY)-offsetY)

		screen.DrawImage(g.TowerImages[g.SelectedTower], drawOpts)
	}
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

	towerImagePaths := []string{
		"asset/basic_tower.png",
		"asset/fast_tower.png",
		"asset/slow_tower.png",
		"asset/heavy_tower.jpg",
		"asset/heavy_tower2.png"}

	// Initialize towerImages first
	towerImages := []*ebiten.Image{}

	// Load tower images
	for _, path := range towerImagePaths {
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			println("Failed to read tower image:", path, err.Error())
			continue
		}
		img, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(data))
		if err != nil {
			println("Failed to decode tower image:", path, err.Error())
			continue
		}
		towerImages = append(towerImages, img)
	}

	// Initialize towers only once
	towers := []*Towers{}

	mythGame := &MythDefense{
		Enemies:          enemies,
		Towers:           towers,
		TowerImages:      towerImages,
		SelectedTower:    -1, // No tower selected initially
		DraggingTower:    false,
		PrevMousePressed: false,
	}

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("Myth Defense - Title Screen")
	ebiten.RunGame(mythGame)
}
