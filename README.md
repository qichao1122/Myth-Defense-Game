# 🏰 Myth Defense Game

A 2D tower defense game built in Go using the [Ebitengine](https://ebitengine.org/) game library. Defend the **Myth Core** — your base — against waves of mythical enemies across three progressively harder maps.

---

## 🎮 What It Does

Myth Defense is a classic tower defense game where players strategically place towers to stop enemies from reaching their base. The game features:

- **3 unique maps** progressing from easy to hard
- **4 tower types**, each with distinct attack behavior, range, damage, and sound effects
- **4 enemy types** with different speeds, health pools, and animated sprites
- **A coin economy** — kill enemies to earn coins, spend them on towers
- **A heart-based health system** — lose a heart every time an enemy reaches the Myth Core
- **Timed rounds** — survive 2 minutes per map to win

---

## 🗺️ Maps & Difficulty

| Map   | Hearts (Lives) | Enemy Spawn Interval | Obstacle Tile | Difficulty |
|-------|---------------|----------------------|---------------|------------|
| Map 1 | ♥♥♥ (3)       | Every 10 seconds     | `tree.png`    | Easy       |
| Map 2 | ♥♥ (2)        | Every 10 seconds     | `sand1.png`   | Easy       |
| Map 3 | ♥ (1)         | Every 5 seconds      | `tree2.png`   | Hard       |

The **Myth Core** base is always placed in the **bottom-right corner** of each map. Enemies spawn from the top-left and pathfind their way toward it.

Maps are loaded from Tiled `.tmx` files (`map1.tmx`, `map2.tmx`, `map3.tmx`) embedded directly into the binary via Go's `embed` package.

---

## 🗼 Towers

All 4 towers are displayed in a panel on the **bottom-left** of the screen with their coin cost. Towers cannot be placed on obstacle tiles or on top of each other — placement updates the pathfinding grid dynamically.

| Tower  | Cost | HP  | Range | Damage | Attack Speed | Bullet Effect        |
|--------|------|-----|-------|--------|--------------|----------------------|
| Basic  | 15   | 100 | 100px | 10     | 1.0/sec      | Standard projectile  |
| Fast   | 10   | 80  | 120px | 15     | 1.5/sec      | High-speed shot      |
| Slow   | 20   | 80  | 90px  | 8      | 0.5/sec      | Applies 5s slow (50% speed reduction) |
| Heavy  | 25   | 150 | 80px  | 25     | 0.8/sec      | High-damage shot     |

Each tower has a **breathing idle animation** and a **recoil/flash effect** when it fires.

---

## 👾 Enemies

Four enemy types spawn simultaneously at each wave interval. All enemies are animated.

| Type       | Speed   | Health | Notes                          |
|------------|---------|--------|--------------------------------|
| Fast       | 120 px/s | 100   | Quick, low health              |
| Slow       | 48 px/s  | 120   | Tanky, plodding                |
| Resistant  | 72 px/s  | 150   | Balanced and durable           |
| Special    | 90 px/s  | 200   | High health, moderate speed    |

Enemies display a **health bar** above them and turn **blue** when affected by a slow tower's projectile.

---

## 💰 Economy

- Players start with **100 coins**
- Each enemy killed awards **10 coins**
- Coins are spent on tower placement
- Towers are grayed out in the UI when the player can't afford them

---

## 🏆 Win / Loss Conditions

**WIN** — Survive the full 2-minute battle timer on all 3 maps without losing all your hearts.  
**LOSE** — Lose all your hearts (enemies reaching the Myth Core deplete hearts one by one).

After winning a map, press **Space** to advance to the next one. Completing all 3 maps plays a special victory sound.

---

## 🔊 Sound Design

| Sound       | Trigger                                 |
|-------------|-----------------------------------------|
| Basic tower | Basic tower fires                       |
| Fast tower  | Fast tower fires                        |
| Slow tower  | Slow tower fires                        |
| Heavy tower | Heavy tower fires                       |
| Game start  | Once at the beginning of the game       |
| Map win     | Completing Map 1 or Map 2               |
| Well done   | Completing all 3 maps (full victory)    |
| Game over   | Losing all hearts                       |

Audio formats supported: `.mp3`, `.ogg` (vorbis), and `.wav`.

---

## ⚙️ How It Works

### Engine & Rendering
The game uses **Ebitengine (v2)**, a 2D game library for Go. The game loop follows Ebitengine's standard `Update()` / `Draw()` pattern running at 60 FPS.

Maps are rendered tile-by-tile from Tiled `.tmx` files using the `go-tiled` library. Each tile image is loaded once and cached in a `map[uint32]*ebiten.Image`.

### Pathfinding (A\*)
Enemies navigate from spawn to the Myth Core using the **A\* algorithm** (`AStar()` in `main.go`). The grid is built from the tile map at load time, with obstacle tiles and placed towers marked as blocked (`1`). When a new tower is placed, the grid is updated and all active enemy paths are recalculated.

The heuristic uses squared Euclidean distance to guide the search. Movement is cardinal only (up, down, left, right — no diagonals).

### Game State
All game state lives in the `MythDefense` struct, including enemies, towers, bullets, coins, health, timers, and the current map index. The `Update()` function advances all subsystems each frame:

1. Enemy movement along their A\* paths
2. Tower targeting and bullet firing (with per-tower cooldown tracking)
3. Bullet movement toward tracked targets (bullets home in if the target moves)
4. Slow effect expiry on enemies
5. Spawn timer triggering new enemy waves
6. Game timer checking for win/loss conditions
7. Mouse input handling for tower drag-and-drop placement

### Tower Placement
Players **click a tower icon** in the bottom-left panel to select it, then **drag and release** to place it on the map. The game checks that the placement tile is walkable before committing — the tower is rejected if placed on an obstacle or out of bounds.

### Assets
All assets (tile maps, images, sounds) are embedded into the binary at compile time using Go's `//go:embed asset/*` directive, so no external files need to accompany the executable.

---

## 🛠️ Tech Stack

| Component       | Library / Tool                                |
|-----------------|-----------------------------------------------|
| Language        | Go                                            |
| Game engine     | [Ebitengine v2](https://ebitengine.org/)      |
| Map format      | [Tiled](https://www.mapeditor.org/) `.tmx`    |
| Map parser      | `github.com/lafriks/go-tiled`                 |
| Camera (unused) | `github.com/tducasse/ebiten-camera`           |
| Asset embedding | Go standard `embed` package                   |
| Audio           | Ebitengine audio (`mp3`, `vorbis`, `wav`)     |

> **Note:** The camera library (`ebiten-camera`) is imported but not actively used in the current version. The author noted this as a known limitation.

---

## 🚀 Running the Game

```bash
git clone https://github.com/qichao1122/Myth-Defense-Game.git
cd Myth-Defense-Game
go run main.go
```

**Requirements:** Go 1.18+ and the dependencies listed in `go.mod`.

---

## 📁 Project Structure

```
Myth-Defense-Game/
├── main.go          # All game logic (single-file architecture)
├── go.mod           # Go module definition
├── go.sum           # Dependency checksums
└── asset/           # Embedded game assets
    ├── map1.tmx     # Easy map 1
    ├── map2.tmx     # Easy map 2
    ├── map3.tmx     # Hard map 3
    ├── *.png        # Tile sheets, sprites, UI icons
    └── *.mp3/.wav   # Sound effects
```

---

*Created by Qichao Zhang.*
