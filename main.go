package main

import (
    "flag"
    "fmt"
    "image"
    "image/jpeg"
    "image/png"
    _ "image/gif"
    "log"
    "math"
    "os"
    "path/filepath"
    "sort"
    "strings"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
    "golang.org/x/image/webp"
    _ "golang.org/x/image/bmp"
)

func isSupportedExt(path string) bool {
    ext := strings.ToLower(filepath.Ext(path))
    switch ext {
    case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif":
        return true
    default:
        return false
    }
}

func loadImage(path string) (*ebiten.Image, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var img image.Image
    switch strings.ToLower(filepath.Ext(path)) {
    case ".webp":
        img, err = webp.Decode(f)
    case ".png":
        img, err = png.Decode(f)
    case ".jpg", ".jpeg":
        img, err = jpeg.Decode(f)
    default:
        img, _, err = image.Decode(f)
    }
    if err != nil {
        return nil, fmt.Errorf("decoding %s: %v", path, err)
    }
    return ebiten.NewImageFromImage(img), nil
}

type Game struct {
    images     []*ebiten.Image
    paths      []string
    idx        int
    fullscreen bool

    savedWinW int
    savedWinH int
}

func (g *Game) Update() error {
    if len(g.images) == 0 {
        return nil
    }

    // Quit keys
    if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
        os.Exit(0)
    }

    // Next / Prev keys
    if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
        g.idx = (g.idx + 1) % len(g.images)
    }
    if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
        g.idx--
        if g.idx < 0 {
            g.idx = len(g.images) - 1
        }
    }

    // Toggle fullscreen / fit
    if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
        g.fullscreen = !g.fullscreen
        if g.fullscreen {
            g.savedWinW, g.savedWinH = ebiten.WindowSize()
            ebiten.SetFullscreen(true)
        } else {
            ebiten.SetFullscreen(false)
            if g.savedWinW > 0 && g.savedWinH > 0 {
                ebiten.SetWindowSize(g.savedWinW, g.savedWinH)
            }
        }
    }

    return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
    if len(g.images) == 0 {
        return
    }
    img := g.images[g.idx]
    iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
    w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

    var scale float64
    if g.fullscreen {
        scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
    } else {
        if iw > w || ih > h {
            scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
        } else {
            scale = 1
        }
    }

    op := &ebiten.DrawImageOptions{}
    op.GeoM.Scale(scale, scale)
    sw, sh := float64(iw)*scale, float64(ih)*scale
    op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

    screen.DrawImage(img, op)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
    return outsideWidth, outsideHeight
}

func collectImages(args []string) ([]string, error) {
    var list []string
    for _, p := range args {
        info, err := os.Stat(p)
        if err != nil {
            return nil, err
        }
        if info.IsDir() {
            err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
                if err != nil {
                    return err
                }
                if fi.IsDir() {
                    return nil
                }
                if isSupportedExt(path) {
                    list = append(list, path)
                }
                return nil
            })
            if err != nil {
                return nil, err
            }
        } else {
            if isSupportedExt(p) {
                list = append(list, p)
            }
        }
    }
    sort.Strings(list)
    return list, nil
}

func main() {
    flag.Parse()
    paths, err := collectImages(flag.Args())
    if err != nil {
        log.Fatal(err)
    }
    if len(paths) == 0 {
        log.Fatal("no image files specified")
    }

    var images []*ebiten.Image
    for _, p := range paths {
        img, err := loadImage(p)
        if err != nil {
            log.Printf("failed to load %s: %v", p, err)
            continue
        }
        images = append(images, img)
    }
    if len(images) == 0 {
        log.Fatal("no images could be loaded")
    }

    g := &Game{
        images: images,
        paths:  paths,
        idx:    0,
    }

    ebiten.SetWindowTitle("Ebiten Image Viewer")
    ebiten.SetWindowSize(800, 600)
    ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

    if err := ebiten.RunGame(g); err != nil {
        log.Fatal(err)
    }
}
