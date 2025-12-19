package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/lafriks/go-tiled"
	camera "github.com/tducasse/ebiten-camera"
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
	audioContext    *audio.Context

	// Sound effects for towers
	basicSound []byte
	fastSound  []byte
	slowSound  []byte
	heavySound []byte

	// Game event sounds
	gameStartSound []byte
	gameOverSound  []byte
	wellDoneSound  []byte
	mapWinSound    []byte
)

type MythDefense struct {
	Enemies          []*Enemies
	EnemyImage       []*ebiten.Image
	Base             *Base
	Towers           []*Towers
	TowerImages      []*ebiten.Image          // Available tower types
	BulletImages     map[string]*ebiten.Image // Bullet images by type
	Bullets          []*Bullet                // Active bullets
	SelectedTower    int                      // Index of selected tower (-1 = none)
	DraggingTower    bool                     // Whether currently dragging a tower
	DragX, DragY     int                      // Current drag position
	PrevMousePressed bool                     // Previous frame mouse state

	SpawnTimer    float64
	SpawnInterval float64
	GameTimer     float64 // Total game time elapsed
	GameDuration  float64 // Total game duration (2 minutes = 120 seconds)
	GameOver      bool    // Game over flag
	GameWon       bool    // Whether player won or lost
	CurrentMap    int     // Current map index (0, 1, or 2)
	AllMaps       []*tiled.Map
	AllTiles      []map[uint32]*ebiten.Image
	EnemyPath     []Node // Store the path for spawning

	// Coin system
	Coins     int           // Current number of coins
	CoinImage *ebiten.Image // Coin icon

	// Pathfinding grid
	Grid [][]int // Grid for pathfinding (0 = walkable, 1 = blocked)

	// Game event audio players
	GameStartPlayer *audio.Player
	GameOverPlayer  *audio.Player
	WellDonePlayer  *audio.Player
	MapWinPlayer    *audio.Player
	GameStarted     bool // Track if game start sound has played

	// Health system
	PlayerHealth int           // Current player health (hearts)
	MaxHealth    int           // Maximum health for current map
	HeartImage   *ebiten.Image // Heart icon

	// Camera system
	Camera     *camera.Camera
	WorldImage *ebiten.Image // World image for rendering
}

type Enemies struct {
	X, Y      float64
	Speed     float64
	Img       *ebiten.Image
	Type      string
	Path      []Node // A* path
	PathIndex int
	Health    int
	MaxHealth int
	Scale     float64

	// Slow effect
	BaseSpeed      float64 // Original speed
	SlowDuration   float64 // Time remaining for slow effect
	SlowMultiplier float64 // Speed multiplier when slowed (e.g., 0.5 = 50% speed)

	// Animation
	AnimFrame  int     // Current animation frame
	AnimTimer  float64 // Time accumulator for animation
	AnimSpeed  float64 // Frames per second
	FrameCount int     // Total number of frames (if using sprite sheet)
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
	MaxHealth   int
	BulletType  string        // Type of bullet this tower shoots
	AttackSound *audio.Player // Sound effect for tower attack

	// Animation
	AnimFrame   int     // Current animation frame
	AnimTimer   float64 // Time accumulator for animation
	AnimSpeed   float64 // Frames per second
	FrameCount  int     // Total number of frames
	IsAttacking bool    // Whether tower is currently attacking
}

type Base struct {
	X, Y       float64
	BaseImgage *ebiten.Image
	BaseHealth int
	MaxHealth  int
}

type Bullet struct {
	X, Y       float64
	TargetX    float64
	TargetY    float64
	Speed      float64
	Damage     int
	Img        *ebiten.Image
	Target     *Enemies
	Active     bool
	BulletType string // Track bullet type for special effects
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

func createGridFromMap(mapData *tiled.Map, mapIndex int) [][]int {
	grid := make([][]int, mapData.Height)
	for i := range grid {
		grid[i] = make([]int, mapData.Width)
	}

	// Initialize all cells as walkable
	for y := 0; y < mapData.Height; y++ {
		for x := 0; x < mapData.Width; x++ {
			grid[y][x] = 0
		}
	}

	// Mark obstacle tiles as unwalkable based on tile ID
	for y := 0; y < mapData.Height; y++ {
		for x := 0; x < mapData.Width; x++ {
			tile := mapData.Layers[0].Tiles[y*mapData.Width+x]
			if tile.Tileset == nil {
				continue
			}

			// Define obstacle tile IDs for each map
			var isObstacle bool

			switch mapIndex {
			case 0: // Map 1 - tree.png
				// Check if this tile is a tree
				// You need to find the tile ID that corresponds to tree.png
				isObstacle = isTileObstacle(mapData, tile, "tree.png")
			case 1: // Map 2 - sand1.png
				// Check if this tile is sand1
				isObstacle = isTileObstacle(mapData, tile, "sand1.png")
			case 2: // Map 3 - tree2.png
				isObstacle = isTileObstacle(mapData, tile, "tree2.png")
			}

			if isObstacle {
				grid[y][x] = 1 // Mark as blocked
			}
		}
	}

	return grid
}

// Helper function to check if a tile is an obstacle based on its source image
func isTileObstacle(mapData *tiled.Map, tile *tiled.LayerTile, obstacleImageName string) bool {
	if tile.Tileset == nil {
		return false
	}

	// Check if the tileset's source image matches the obstacle image
	if tile.Tileset.Image != nil && tile.Tileset.Image.Source == obstacleImageName {
		return true
	}

	return false
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

		// Add pulsing/breathing animation effect
		pulse := 1.0 + math.Sin(float64(enemy.AnimFrame)*0.2)*0.05 // Small pulsing effect

		drawOpts.GeoM.Scale(enemy.Scale*pulse, enemy.Scale*pulse)
		drawOpts.GeoM.Translate(enemy.X, enemy.Y)

		// Add color tint if slowed
		if enemy.SlowDuration > 0 {
			drawOpts.ColorScale.Scale(0.7, 0.7, 1.2, 1.0) // Blue tint when slowed
		}

		screen.DrawImage(enemy.Img, drawOpts)

		// Draw health bar
		drawEnemyHealthBar(screen, enemy)
	}
}

func drawEnemyHealthBar(screen *ebiten.Image, e *Enemies) {
	if e.MaxHealth <= 0 {
		return
	}

	barWidth := 32.0
	barHeight := 4.0

	healthRatio := float64(e.Health) / float64(e.MaxHealth)
	if healthRatio < 0 {
		healthRatio = 0
	}

	x := e.X
	y := e.Y - 6 // draw slightly above enemy

	// Background (red)
	ebitenutil.DrawRect(
		screen,
		x,
		y,
		barWidth,
		barHeight,
		color.RGBA{200, 0, 0, 255},
	)

	// Foreground (green)
	ebitenutil.DrawRect(
		screen,
		x,
		y,
		barWidth*healthRatio,
		barHeight,
		color.RGBA{0, 200, 0, 255},
	)
}

func drawTowers(screen *ebiten.Image, towers []*Towers) {
	for _, tower := range towers {
		if tower.Img != nil {
			drawOpts := &ebiten.DrawImageOptions{}

			// Add animation effects
			scale := 0.5

			// If tower is attacking or on cooldown, add a "firing" effect
			if tower.Cooldown > 0 && tower.Cooldown > (1.0/tower.AttackSpeed)-0.2 {
				// Tower just fired - add recoil effect
				recoil := (1.0/tower.AttackSpeed - tower.Cooldown) * 5.0
				scale += recoil * 0.05

				// Add flash effect(animated effect)
				drawOpts.ColorScale.Scale(1.2, 1.2, 1.2, 1.0)
			}

			// Add idle breathing animation
			breathe := 1.0 + math.Sin(float64(tower.AnimFrame)*0.1)*0.02
			scale *= breathe

			drawOpts.GeoM.Scale(scale, scale)
			drawOpts.GeoM.Translate(tower.X, tower.Y)
			screen.DrawImage(tower.Img, drawOpts)
		}
	}
}

// draws the tower selection panel in the bottom-left
func drawTowerSelectionUI(screen *ebiten.Image, game *MythDefense) {
	if len(game.TowerImages) == 0 {
		return
	}

	// Tower costs
	towerCosts := []int{15, 10, 20, 25, 25} // basic, fast, slow, heavy, heavy2

	// UI settings
	iconSize := 60
	padding := 10
	startX := padding // Left side
	startY := windowHeight - (len(game.TowerImages)*(iconSize+padding) + padding)

	// Draw background panel
	for i := 0; i < len(game.TowerImages); i++ {
		y := startY + i*(iconSize+padding)

		// Check if player can afford this tower
		canAfford := i < len(towerCosts) && game.Coins >= towerCosts[i]

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

			// Gray out if can't afford
			if !canAfford {
				drawOpts.ColorScale.Scale(0.5, 0.5, 0.5, 1.0)
			}

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

		// Draw cost text
		if i < len(towerCosts) {
			costText := fmt.Sprintf("%d", towerCosts[i])
			ebitenutil.DebugPrintAt(screen, costText, startX+iconSize+5, y+iconSize/2)

			// Draw coin icon next to cost
			if game.CoinImage != nil {
				coinOpts := &ebiten.DrawImageOptions{}
				coinScale := 0.15
				coinOpts.GeoM.Scale(coinScale, coinScale)
				coinOpts.GeoM.Translate(float64(startX+iconSize+25), float64(y+iconSize/2-5))
				screen.DrawImage(game.CoinImage, coinOpts)
			}
		}
	}
}

// handleTowerSelection checks if mouse click is on tower selection UI
func (g *MythDefense) handleTowerSelection(x, y int) bool {
	// Tower costs
	towerCosts := []int{15, 10, 20, 25, 25} // basic, fast, slow, heavy, heavy2

	iconSize := 60
	padding := 10
	startX := padding // Left side
	startY := windowHeight - (len(g.TowerImages)*(iconSize+padding) + padding)

	for i := 0; i < len(g.TowerImages); i++ {
		iconY := startY + i*(iconSize+padding)

		if x >= startX && x <= startX+iconSize && y >= iconY && y <= iconY+iconSize {
			// Check if player can afford this tower
			if i < len(towerCosts) && g.Coins >= towerCosts[i] {
				g.SelectedTower = i
				return true
			}

			return false
		}
	}
	return false
}

// placeTower places a tower at the specified position
func (g *MythDefense) placeTower(x, y float64) {
	if g.SelectedTower < 0 || g.SelectedTower >= len(g.TowerImages) {
		return
	}

	// Tower costs
	towerCosts := []int{15, 10, 20, 25, 25} // basic, fast, slow, heavy, heavy2

	// Check if player can afford the tower
	if g.SelectedTower >= len(towerCosts) || g.Coins < towerCosts[g.SelectedTower] {
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

	// Validate placement is within map bounds AND grid bounds
	mapWidth := float64(titleMap.Width * titleMap.TileWidth)
	mapHeight := float64(titleMap.Height * titleMap.TileHeight)

	// Check the map bounds first
	if placementX < 0 || placementY < 0 || placementX >= mapWidth || placementY >= mapHeight {
		return
	}

	// Check grid bounds (convert to grid coordinates and validate)
	gridX := int(placementX+16) / 32
	gridY := int(placementY+16) / 32

	if gridX < 0 || gridY < 0 || gridY >= len(g.Grid) || gridX >= len(g.Grid[0]) {
		// Tower would be outside grid bounds, don't place it
		return
	}

	// Create tower with stats based on type
	var tower *Towers
	switch g.SelectedTower {
	case 0: // Basic tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[0], TowerHealth: 100, MaxHealth: 100, Range: 100, Damage: 10, AttackSpeed: 1.0, BulletType: "basic", AttackSound: createAudioPlayer(basicSound), AnimSpeed: 10.0, FrameCount: 1}
	case 1: // Fast tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[1], TowerHealth: 80, MaxHealth: 80, Range: 120, Damage: 15, AttackSpeed: 1.5, BulletType: "fast", AttackSound: createAudioPlayer(fastSound), AnimSpeed: 15.0, FrameCount: 1}
	case 2: // Slow tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[2], TowerHealth: 80, MaxHealth: 80, Range: 90, Damage: 8, AttackSpeed: 0.5, BulletType: "slow", AttackSound: createAudioPlayer(slowSound), AnimSpeed: 8.0, FrameCount: 1}
	case 3: // Heavy tower
		tower = &Towers{X: placementX, Y: placementY, Img: g.TowerImages[3], TowerHealth: 150, MaxHealth: 150, Range: 80, Damage: 25, AttackSpeed: 0.8, BulletType: "heavy", AttackSound: createAudioPlayer(heavySound), AnimSpeed: 12.0, FrameCount: 1}

	}

	if tower != nil {
		g.Towers = append(g.Towers, tower)
		// Deduct cost
		g.Coins -= towerCosts[g.SelectedTower]

		// Update grid and recalculate paths for enemies
		g.updateGridWithTowers()
		g.recalculateEnemyPaths()
	}
}

func drawBase(screen *ebiten.Image, base *Base) {
	if base == nil || base.BaseImgage == nil {
		return
	}

	drawOpts := &ebiten.DrawImageOptions{}
	drawOpts.GeoM.Scale(0.5, 0.5) // Add scale to make base visible
	drawOpts.GeoM.Translate(base.X, base.Y)
	screen.DrawImage(base.BaseImgage, drawOpts)

	// Draw "Myth Core" text above the base
	mythCoreText := "Myth Core"
	// Position text centered above the base
	textX := int(base.X) + 20 // Adjust for centering
	textY := int(base.Y) - 15 // Position above the base
	ebitenutil.DebugPrintAt(screen, mythCoreText, textX, textY)
}

func drawBullets(screen *ebiten.Image, bullets []*Bullet) {
	for _, bullet := range bullets {
		if !bullet.Active || bullet.Img == nil {
			continue
		}
		drawOpts := &ebiten.DrawImageOptions{}
		drawOpts.GeoM.Scale(0.5, 0.5) // Increase scale to make bullets more visible
		drawOpts.GeoM.Translate(bullet.X, bullet.Y)
		screen.DrawImage(bullet.Img, drawOpts)
	}
}

func UpdateEnemies(enemies []*Enemies, dt float64) {
	for _, e := range enemies {
		// Update animation
		e.AnimTimer += dt
		if e.AnimTimer >= 1.0/e.AnimSpeed {
			e.AnimTimer = 0
			e.AnimFrame++
			if e.AnimFrame >= 60 { // Reset after 60 frames
				e.AnimFrame = 0
			}
		}

		// Update slow effect
		if e.SlowDuration > 0 {
			e.SlowDuration -= dt
			if e.SlowDuration <= 0 {
				// Slow effect expired, restore original speed
				e.Speed = e.BaseSpeed
			}
		}

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

func spawnEnemy(g *MythDefense, path []Node, enemyImages []*ebiten.Image) {
	if len(enemyImages) == 0 {
		return
	}

	// Spawn one of each enemy type at each interval
	enemyTypes := []struct {
		imgIndex  int
		typeName  string
		speed     float64
		health    int
		maxHealth int
		scale     float64
	}{
		{0, "fast", 120.0, 100, 100, 0.25},
		{1, "slow", 48.0, 120, 120, 0.5},
		{2, "resistant", 72.0, 150, 150, 0.5},
		{3, "special", 90.0, 200, 200, 0.5},
	}

	for _, et := range enemyTypes {
		if et.imgIndex >= len(enemyImages) {
			continue
		}

		enemy := &Enemies{
			X:              0,
			Y:              0,
			Img:            enemyImages[et.imgIndex],
			Type:           et.typeName,
			Speed:          et.speed,
			BaseSpeed:      et.speed, // Store original speed
			Health:         et.health,
			MaxHealth:      et.maxHealth,
			Path:           path,
			PathIndex:      0,
			Scale:          et.scale,
			SlowDuration:   0.0,
			SlowMultiplier: 1.0,
		}

		g.Enemies = append(g.Enemies, enemy)
	}
}

func UpdateBullets(bullets []*Bullet, enemies []*Enemies, dt float64) {
	for i := len(bullets) - 1; i >= 0; i-- {
		bullet := bullets[i]
		if !bullet.Active {
			continue
		}

		// Check if target enemy still exists and is alive
		targetAlive := false
		if bullet.Target != nil {
			for _, e := range enemies {
				if e == bullet.Target && e.Health > 0 {
					targetAlive = true
					// Update target position
					bullet.TargetX = e.X + 16 // Center of enemy
					bullet.TargetY = e.Y + 16
					break
				}
			}
		}

		// If target is dead, deactivate bullet
		if !targetAlive {
			bullet.Active = false
			continue
		}

		// Move bullet towards target
		dx := bullet.TargetX - bullet.X
		dy := bullet.TargetY - bullet.Y
		distance := math.Hypot(dx, dy)

		// Check if bullet reached target
		if distance < bullet.Speed*dt {
			// Hit the target
			if bullet.Target != nil {
				bullet.Target.Health -= bullet.Damage

				// Apply slow effect if this is a slow bullet
				if bullet.BulletType == "slow" {
					bullet.Target.SlowDuration = 5.0   // Slow for 5 seconds
					bullet.Target.SlowMultiplier = 0.5 // 50% speed
					bullet.Target.Speed = bullet.Target.BaseSpeed * bullet.Target.SlowMultiplier
				}
			}
			bullet.Active = false
		} else {
			// Move bullet
			bullet.X += (dx / distance) * bullet.Speed * dt
			bullet.Y += (dy / distance) * bullet.Speed * dt
		}
	}
}

func UpdateTowers(towers []*Towers, enemies []*Enemies, bullets *[]*Bullet, bulletImages map[string]*ebiten.Image, frame_time float64) {
	for _, tower := range towers {
		// Update tower animation
		tower.AnimTimer += frame_time
		if tower.AnimTimer >= 1.0/tower.AnimSpeed {
			tower.AnimTimer = 0
			tower.AnimFrame++
			if tower.AnimFrame >= 60 {
				tower.AnimFrame = 0
			}
		}

		// Always decrease cooldown
		if tower.Cooldown > 0 {
			tower.Cooldown -= frame_time
		}

		// Only shoot if cooldown is finished
		if tower.Cooldown > 0 {
			continue
		}

		// Find closest enemy in range
		var closest *Enemies
		minDistance := tower.Range
		for _, e := range enemies {
			if e.Health <= 0 {
				continue
			}

			// Calculate distance from tower center
			towerCenterX := tower.X + 16 // Approximate tower center
			towerCenterY := tower.Y + 16

			dx := e.X - towerCenterX
			dy := e.Y - towerCenterY
			distance := math.Hypot(dx, dy)

			if distance <= minDistance {
				closest = e
				minDistance = distance
			}
		}

		// If enemy found, shoot bullet
		if closest != nil {
			tower.IsAttacking = true

			// Play attack sound
			if tower.AttackSound != nil {
				tower.AttackSound.Rewind()
				tower.AttackSound.Play()
			}

			bulletImg := bulletImages[tower.BulletType]
			if bulletImg != nil {
				towerCenterX := tower.X + 16
				towerCenterY := tower.Y + 16

				// Determine bullet speed based on type
				var bulletSpeed float64
				switch tower.BulletType {
				case "fast":
					bulletSpeed = 300.0
				case "slow":
					bulletSpeed = 120.0
				case "heavy":
					bulletSpeed = 180.0
				case "basic":
					bulletSpeed = 240.0
				default:
					bulletSpeed = 240.0
				}

				bullet := &Bullet{
					X:          towerCenterX,
					Y:          towerCenterY,
					TargetX:    closest.X + 16,
					TargetY:    closest.Y + 16,
					Speed:      bulletSpeed,
					Damage:     tower.Damage,
					Img:        bulletImg,
					Target:     closest,
					Active:     true,
					BulletType: tower.BulletType, // Track bullet type
				}
				*bullets = append(*bullets, bullet)
			}
			// Reset cooldown after shooting
			tower.Cooldown = 1.0 / tower.AttackSpeed
		} else {
			tower.IsAttacking = false
		}
	}
}

func (g *MythDefense) Update() error {
	// Play game start sound once at the beginning
	if !g.GameStarted && g.GameStartPlayer != nil {
		g.GameStartPlayer.Rewind()
		g.GameStartPlayer.Play()
		g.GameStarted = true
	}

	// Check if game is over
	if g.GameOver {
		// Check for space key to proceed to next map after winning
		if g.GameWon && ebiten.IsKeyPressed(ebiten.KeySpace) {
			g.loadNextMap()
		}
		return nil // Stop updating if game is over
	}

	deltaTime := 1.0 / 60.0 // Proper frame time (assuming 60 FPS)
	UpdateEnemies(g.Enemies, deltaTime)
	UpdateTowers(g.Towers, g.Enemies, &g.Bullets, g.BulletImages, deltaTime)
	UpdateBullets(g.Bullets, g.Enemies, deltaTime)

	// Check if any enemy reached the base
	if g.Base != nil {
		for i := len(g.Enemies) - 1; i >= 0; i-- {
			e := g.Enemies[i]
			if e.Path != nil && e.PathIndex >= len(e.Path) {
				// Enemy reached the end of path (base location)
				g.PlayerHealth--

				// Remove the enemy that reached the base
				g.Enemies = append(g.Enemies[:i], g.Enemies[i+1:]...)

				// Check if player is out of health
				if g.PlayerHealth <= 0 {
					g.GameOver = true
					g.GameWon = false
					// Play game over sound
					if g.GameOverPlayer != nil {
						g.GameOverPlayer.Rewind()
						g.GameOverPlayer.Play()
					}
					return nil
				}
			}
		}
	}

	// Remove dead enemies and award coins
	aliveEnemies := []*Enemies{}
	for _, e := range g.Enemies {
		if e.Health > 0 {
			aliveEnemies = append(aliveEnemies, e)
		} else {
			// Enemy died - award 10 coins
			g.Coins += 10
		}
	}
	g.Enemies = aliveEnemies

	// Remove inactive bullets
	activeBullets := []*Bullet{}
	for _, b := range g.Bullets {
		if b.Active {
			activeBullets = append(activeBullets, b)
		}
	}
	g.Bullets = activeBullets

	// === Enemy spawn timer ===
	g.SpawnTimer += deltaTime
	if g.SpawnTimer >= g.SpawnInterval {
		g.SpawnTimer -= g.SpawnInterval // Subtract interval instead of resetting to 0

		// Spawn all enemy types at once using the stored path
		if len(g.EnemyImage) > 0 && g.EnemyPath != nil && len(g.EnemyPath) > 0 {
			spawnEnemy(g, g.EnemyPath, g.EnemyImage)
		}
	}

	// === Game timer ===
	g.GameTimer += deltaTime
	if g.GameTimer >= g.GameDuration {
		// Game over - time's up! Player survived
		g.GameOver = true
		g.GameWon = true

		// Play appropriate win sound based on map
		if g.CurrentMap == 0 || g.CurrentMap == 1 {
			// Map 1 or Map 2 completed - play mapwin sound
			if g.MapWinPlayer != nil {
				g.MapWinPlayer.Rewind()
				g.MapWinPlayer.Play()
			}
		}
		// Note: welldone sound will play when all maps are completed in loadNextMap()

		return nil
	}

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
	drawBase(screen, g.Base) // Draw base before enemies and towers so it's visible
	drawEnemies(screen, g.Enemies)
	drawTowers(screen, g.Towers)
	drawBullets(screen, g.Bullets)
	drawTowerSelectionUI(screen, g)

	// Draw coin counter in top-right corner
	coinText := fmt.Sprintf("Coins: %d", g.Coins)
	ebitenutil.DebugPrintAt(screen, coinText, windowWidth-120, 10)

	// Draw coin icon next to counter
	if g.CoinImage != nil {
		coinOpts := &ebiten.DrawImageOptions{}
		coinScale := 0.2
		coinOpts.GeoM.Scale(coinScale, coinScale)
		coinOpts.GeoM.Translate(float64(windowWidth-135), 8)
		screen.DrawImage(g.CoinImage, coinOpts)
	}

	// Draw hearts in top-right corner (below coins)
	if g.HeartImage != nil {
		heartScale := 0.15
		heartSize := 30 // Approximate size after scaling
		startX := windowWidth - 120
		startY := 35

		for i := 0; i < g.PlayerHealth; i++ {
			heartOpts := &ebiten.DrawImageOptions{}
			heartOpts.GeoM.Scale(heartScale, heartScale)
			heartOpts.GeoM.Translate(float64(startX+i*heartSize), float64(startY))
			screen.DrawImage(g.HeartImage, heartOpts)
		}
	}

	// Draw game timer
	remainingTime := g.GameDuration - g.GameTimer
	if remainingTime < 0 {
		remainingTime = 0
	}
	minutes := int(remainingTime) / 60
	seconds := int(remainingTime) % 60
	timerText := fmt.Sprintf("Time: %02d:%02d", minutes, seconds)
	ebitenutil.DebugPrintAt(screen, timerText, 10, 10)

	// Draw current map indicator
	mapText := fmt.Sprintf("Map: %d/%d", g.CurrentMap+1, len(g.AllMaps))
	ebitenutil.DebugPrintAt(screen, mapText, 10, 30)

	// Draw difficulty mode
	var modeText string
	if g.CurrentMap == 0 || g.CurrentMap == 1 {
		modeText = "Easy Mode"
	} else if g.CurrentMap == 2 {
		modeText = "Hard Mode"
	}
	ebitenutil.DebugPrintAt(screen, modeText, 10, 50)

	// Draw game over message
	if g.GameOver {
		// Draw semi-transparent overlay
		ebitenutil.DrawRect(screen, 0, 0, float64(windowWidth), float64(windowHeight),
			color.RGBA{0, 0, 0, 180})

		// Draw game over text
		var message string
		if g.GameWon {
			if g.CurrentMap+1 >= len(g.AllMaps) {
				message = "CONGRATULATIONS!\nYou completed all maps!"
			} else {
				message = fmt.Sprintf("YOU WON!\nYou survived map %d!\n\nPress SPACE for next map", g.CurrentMap+1)
			}
		} else {
			message = "GAME OVER!\nEnemies reached your base!"
		}
		ebitenutil.DebugPrintAt(screen, message, windowWidth/2-100, windowHeight/2)
	}

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

// createAudioPlayer creates an audio player from sound data
func createAudioPlayer(soundData []byte) *audio.Player {
	if soundData == nil || audioContext == nil {
		return nil
	}

	// WAV format
	wavStream, err := wav.DecodeWithSampleRate(audioContext.SampleRate(), bytes.NewReader(soundData))
	if err == nil {
		player, err := audioContext.NewPlayer(wavStream)
		if err == nil {
			return player
		}
	}

	// MP3 format
	mp3Stream, err := mp3.DecodeWithSampleRate(audioContext.SampleRate(), bytes.NewReader(soundData))
	if err == nil {
		player, err := audioContext.NewPlayer(mp3Stream)
		if err == nil {
			return player
		}
	}

	// OGG format
	vorbisStream, err := vorbis.DecodeWithSampleRate(audioContext.SampleRate(), bytes.NewReader(soundData))
	if err == nil {
		player, err := audioContext.NewPlayer(vorbisStream)
		if err == nil {
			return player
		}
	}

	println("Failed to decode audio in any supported format (WAV, MP3, OGG)")
	return nil
}

func main() {
	// Initialize audio context
	audioContext = audio.NewContext(48000)

	// Load sound effects
	var err error
	basicSound, err = assetsFS.ReadFile("asset/basic.mp3.wav")
	if err != nil {
		println("Failed to load basic.mp3.wav:", err.Error())
	}

	fastSound, err = assetsFS.ReadFile("asset/fast.mp3.mp3")
	if err != nil {
		println("Failed to load fast.mp3:", err.Error())
	}

	slowSound, err = assetsFS.ReadFile("asset/slow.mp3.mp3")
	if err != nil {
		println("Failed to load slow.mp3:", err.Error())
	}

	heavySound, err = assetsFS.ReadFile("asset/heavy.mp3.wav")
	if err != nil {
		println("Failed to load heavy.mp3.wav:", err.Error())
	}

	// Load game event sounds
	gameStartSound, err = assetsFS.ReadFile("asset/gamestart.mp3.mp3")
	if err != nil {
		println("Failed to load gamestart.mp3:", err.Error())
	}

	gameOverSound, err = assetsFS.ReadFile("asset/gameover.mp3.mp3")
	if err != nil {
		println("Failed to load gameover.mp3:", err.Error())
	}

	wellDoneSound, err = assetsFS.ReadFile("asset/welldone.mp3")
	if err != nil {
		println("Failed to load welldone.mp3:", err.Error())
	}

	mapWinSound, err = assetsFS.ReadFile("asset/mapwin.mp3")
	if err != nil {
		println("Failed to load mapwin.mp3:", err.Error())
	}

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

	// Load coin image
	var coinImage *ebiten.Image
	coinImgData, err := assetsFS.ReadFile("asset/coin.png")
	if err != nil {
		println("Failed to read coin image:", err.Error())
	} else {
		coinImage, _, err = ebitenutil.NewImageFromReader(bytes.NewReader(coinImgData))
		if err != nil {
			println("Failed to decode coin image:", err.Error())
		}
	}

	// Load heart image
	var heartImage *ebiten.Image
	heartImgData, err := assetsFS.ReadFile("asset/heart.png")
	if err != nil {
		println("Failed to read heart image:", err.Error())
	} else {
		heartImage, _, err = ebitenutil.NewImageFromReader(bytes.NewReader(heartImgData))
		if err != nil {
			println("Failed to decode heart image:", err.Error())
		}
	}

	// Load base image with error handling
	var baseImage *ebiten.Image
	baseImgData, err := assetsFS.ReadFile("asset/castle.png")
	if err != nil {
		println("Failed to read base image:", err.Error())
	} else {
		baseImage, _, err = ebitenutil.NewImageFromReader(bytes.NewReader(baseImgData))
		if err != nil {
			println("Failed to decode base image:", err.Error())
		}
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
		grid = createGridFromMap(titleMap, 0)
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

	towerImagePaths := []string{
		"asset/basic_tower.png",
		"asset/fast_tower.png",
		"asset/slow_tower.png",
		"asset/heavy_tower.jpg",
	}

	towerImages := []*ebiten.Image{}

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

	bulletImagePaths := map[string]string{
		"basic": "asset/basic_bullet.png",
		"fast":  "asset/fast_bullet.png",
		"slow":  "asset/slow_bullet.png",
		"heavy": "asset/heavy_bullet.png",
	}

	bulletImages := make(map[string]*ebiten.Image)
	for bulletType, path := range bulletImagePaths {
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			println("Failed to read bullet image:", path, err.Error())
			continue
		}
		img, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(data))
		if err != nil {
			println("Failed to decode bullet image:", path, err.Error())
			continue
		}
		bulletImages[bulletType] = img
	}

	towers := []*Towers{}

	var baseWidth, baseHeight float64
	if baseImage != nil {
		bounds := baseImage.Bounds()
		baseWidth = float64(bounds.Dx()) * 0.5
		baseHeight = float64(bounds.Dy()) * 0.5
	}

	base := &Base{
		X:          float64(windowWidth) - baseWidth - 10,
		Y:          float64(windowHeight) - baseHeight - 10,
		BaseImgage: baseImage,
		BaseHealth: 500,
		MaxHealth:  500,
	}

	// Calculate map size for world image - BEFORE using it
	mapWidth := titleMap.Width * titleMap.TileWidth
	mapHeight := titleMap.Height * titleMap.TileHeight
	worldImage := ebiten.NewImage(mapWidth, mapHeight)

	mythGame := &MythDefense{
		Enemies:          enemies,
		EnemyImage:       enemyImages,
		Towers:           towers,
		TowerImages:      towerImages,
		BulletImages:     bulletImages,
		Bullets:          []*Bullet{},
		SelectedTower:    -1,
		DraggingTower:    false,
		PrevMousePressed: false,

		SpawnTimer:    10.0,
		SpawnInterval: 10.0,
		GameTimer:     0.0,
		GameDuration:  60.0,
		GameOver:      false,
		GameWon:       false,

		Base: base,

		CurrentMap: 0,
		AllMaps:    maps,
		AllTiles:   tiles,
		EnemyPath:  path,

		Coins:     100,
		CoinImage: coinImage,

		Grid: grid,

		GameStartPlayer: createAudioPlayer(gameStartSound),
		GameOverPlayer:  createAudioPlayer(gameOverSound),
		WellDonePlayer:  createAudioPlayer(wellDoneSound),
		MapWinPlayer:    createAudioPlayer(mapWinSound),
		GameStarted:     false,

		PlayerHealth: 3,
		MaxHealth:    3,
		HeartImage:   heartImage,

		Camera:     camera.Init(windowWidth, windowHeight),
		WorldImage: worldImage,
	}

	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("Myth Defense - Title Screen")
	ebiten.RunGame(mythGame)
}

// updateGridWithTowers updates the grid to mark tower positions as blocked
func (g *MythDefense) updateGridWithTowers() {
	// Reset grid to walkable (but keep map obstacles)
	if titleMap != nil {
		g.Grid = createGridFromMap(titleMap, g.CurrentMap)
	}

	// Mark tower positions as blocked
	for _, tower := range g.Towers {
		// Convert tower pixel position to grid coordinates
		gridX := int(tower.X+16) / 32 // +16 to get center of tower
		gridY := int(tower.Y+16) / 32

		// Add bounds checking to prevent index out of range
		if gridY >= 0 && gridY < len(g.Grid) && gridX >= 0 && gridX < len(g.Grid[0]) {
			g.Grid[gridY][gridX] = 1 // Mark as blocked
		}
	}
}

// recalculateEnemyPaths recalculates paths for all enemies
func (g *MythDefense) recalculateEnemyPaths() {
	start := Node{X: 0, Y: 0}
	end := Node{X: 20, Y: 20}

	newPath := AStar(g.Grid, start, end)
	if newPath == nil {
		// If no path found, keep old path
		return
	}

	// Update path for all existing enemies
	for _, enemy := range g.Enemies {
		// Find closest node in new path to enemy's current position
		currentGridX := int(enemy.X) / 32
		currentGridY := int(enemy.Y) / 32

		minDist := math.MaxFloat64
		closestIndex := 0

		for i, node := range newPath {
			dx := float64(node.X - currentGridX)
			dy := float64(node.Y - currentGridY)
			dist := dx*dx + dy*dy

			if dist < minDist {
				minDist = dist
				closestIndex = i
			}
		}

		// Update enemy path starting from closest node
		enemy.Path = newPath
		enemy.PathIndex = closestIndex
	}

	// Update the spawn path
	g.EnemyPath = newPath
}

// loadNextMap loads the next map and resets the game state
func (g *MythDefense) loadNextMap() {
	g.CurrentMap++

	// Check if there are more maps available
	if g.CurrentMap >= len(g.AllMaps) {
		// No more maps - game completed! Play welldone sound
		g.GameOver = true
		g.GameWon = true

		// Play welldone sound when all maps are completed
		if g.WellDonePlayer != nil {
			g.WellDonePlayer.Rewind()
			g.WellDonePlayer.Play()
		}
		return
	}

	// Load the next map
	titleMap = g.AllMaps[g.CurrentMap]
	titleTileImages = g.AllTiles[g.CurrentMap]

	// Create new grid and path for the new map with correct map index
	grid := createGridFromMap(titleMap, g.CurrentMap) // Pass current map index
	g.Grid = grid
	start := Node{X: 0, Y: 0}
	end := Node{X: 20, Y: 20}
	path := AStar(grid, start, end)
	if path == nil {
		path = []Node{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 20}}
	}
	g.EnemyPath = path

	// Reset game state
	g.Enemies = []*Enemies{}
	g.Bullets = []*Bullet{}
	g.Towers = []*Towers{}
	g.GameTimer = 0.0
	g.GameOver = false
	g.GameWon = false

	// Set health and spawn interval based on map
	switch g.CurrentMap {
	case 0: // Map 1 - Easy Mode
		g.PlayerHealth = 3
		g.MaxHealth = 3
		g.SpawnInterval = 10.0
		g.SpawnTimer = 10.0
	case 1: // Map 2 - Easy Mode
		g.PlayerHealth = 2
		g.MaxHealth = 2
		g.SpawnInterval = 10.0
		g.SpawnTimer = 10.0
	case 2: // Map 3 - Hard Mode
		g.PlayerHealth = 1
		g.MaxHealth = 1
		g.SpawnInterval = 5.0 // Faster spawn for hard mode
		g.SpawnTimer = 5.0
	}

	// Reset base health
	if g.Base != nil {
		g.Base.BaseHealth = g.Base.MaxHealth
	}
}
