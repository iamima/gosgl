/*Package gosgl is package gosgl
 */
package gosgl

import (
	"container/list"
	"image"
	"io/ioutil"
	"path"
	"runtime"

	"github.com/Jragonmiris/mathgl"
	"github.com/go-gl/gl"
	"github.com/go-gl/glh"
	"github.com/phaikawl/poly2tri-go/p2t"
)

//Drawer represents a program and its buffers
type Drawer struct {
	program gl.Program
	vao     gl.VertexArray
	vbo     gl.Buffer
}

type Canvas struct {
	W, H int
}

func MakeCanvas(w, h int) Canvas {
	return Canvas{w, h}
}

var (
	gQuadraticDrawer          *Drawer
	gTriangleDrawer           *Drawer
	gQuadraticApproxPrecision float32 = 10
)

func lastPt(l []image.Point) image.Point {
	return l[len(l)-1]
}

func Pt(x, y int) image.Point {
	return image.Point{x, y}
}

func NewDrawer(vshader, fshader string) *Drawer {
	vao := gl.GenVertexArray()
	vao.Bind()

	vbo := gl.GenBuffer()
	vbo.Bind(gl.ARRAY_BUFFER)

	vsh := ShaderFromFile(gl.VERTEX_SHADER, vshader)
	fsh := ShaderFromFile(gl.FRAGMENT_SHADER, fshader)

	program := glh.NewProgram(vsh, fsh)
	program.BindFragDataLocation(0, "outColor")
	program.Use()

	return &Drawer{
		program: program,
		vao:     vao,
		vbo:     vbo,
	}
}

func newQuadraticDrawer() *Drawer {
	dr := NewDrawer("vshader.glsl", "quadratic_fshader.glsl")
	program := dr.program
	posAttr := program.GetAttribLocation("position")
	posAttr.AttribPointer(2, gl.FLOAT, false, 4*4, uintptr(0))
	posAttr.EnableArray()

	texAttr := program.GetAttribLocation("texcoord")
	texAttr.AttribPointer(2, gl.FLOAT, false, 4*4, uintptr(8))
	glh.OpenGLSentinel()
	texAttr.EnableArray()

	return dr
}

func newTriangleDrawer() *Drawer {
	dr := NewDrawer("vshader.glsl", "triangle_fshader.glsl")
	program := dr.program
	posAttr := program.GetAttribLocation("position")
	posAttr.AttribPointer(2, gl.FLOAT, false, 2*4, uintptr(0))
	posAttr.EnableArray()

	return dr
}

func (dr *Drawer) activate() {
	dr.vao.Bind()
	dr.vbo.Bind(gl.ARRAY_BUFFER)
	dr.program.Use()
}

func Init() {
	gQuadraticDrawer = newQuadraticDrawer()
	gTriangleDrawer = newTriangleDrawer()

	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Enable(gl.BLEND)
	gl.Enable(gl.STENCIL_TEST)
}

func Point(x, y int) image.Point {
	return image.Point{x, y}
}

type QuadraticCurve struct {
	Points [3]image.Point
}

type BezierCurve struct {
	Points [4]image.Point
}

type PathSegment interface {
	Draw(Canvas)
}

type Path struct {
	Segs      *list.List
	endPoints *list.List
}

func XY(p image.Point) (int, int) {
	return p.X, p.Y
}

func NewPath() *Path {
	p := new(Path)
	p.Segs = new(list.List)
	return p
}

func (p *Path) EndPoint() image.Point {
	return p.endPoints.Back().Value.(image.Point)
}

func (p *Path) NewEnd(pt image.Point) {
	if p.endPoints == nil {
		p.endPoints = new(list.List)
	}
	p.endPoints.PushBack(pt)
}

func (p *Path) StartAt(pt image.Point) *Path {
	p.NewEnd(pt)
	return p
}

func (p *Path) QuadraticTo(p2, c image.Point) *Path {
	p.Segs.PushBack(MakeQuadraticCurve(
		p.EndPoint(),
		c, p2))
	p.NewEnd(p2)
	return p
}

func (p *Path) BezierTo(p2, c1, c2 image.Point) *Path {
	p.Segs.PushBack(MakeBezierCurve(
		p.EndPoint(),
		c1, c2, p2))
	p.NewEnd(p2)
	return p
}

func fill(canv Canvas) {
	gTriangleDrawer.activate()
	gl.ColorMask(true, true, true, true)
	gl.StencilFunc(gl.EQUAL, 1, 0xff)
	w, h := canv.W, canv.H
	p := canv.toGLPoints([]image.Point{
		Pt(0, 0),
		Pt(w, 0),
		Pt(w, h),
		Pt(0, h),
	})
	vertices := []float32{
		p[0].X, p[0].Y,
		p[1].X, p[1].Y,
		p[3].X, p[3].Y,
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, vertices, gl.STATIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)

	vertices = []float32{
		p[1].X, p[1].Y,
		p[2].X, p[2].Y,
		p[3].X, p[3].Y,
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, vertices, gl.STATIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
}

func (p *Path) Draw(canv Canvas) {
	gl.ClearStencil(0)
	gl.Clear(gl.STENCIL_BUFFER_BIT)
	gl.StencilMask(0x01)
	gl.StencilFunc(gl.ALWAYS, 0, 0xff)
	gl.StencilOp(gl.KEEP, gl.KEEP, gl.INVERT)
	gl.ColorMask(false, false, false, false)

	p.draw(canv)
	fill(canv)
}

func (p *Path) draw(canv Canvas) {
	for e := p.Segs.Front(); e != nil; e = e.Next() {
		e.Value.(PathSegment).Draw(canv)
	}

	if p.endPoints.Back().Value.(image.Point) == p.endPoints.Front().Value.(image.Point) {
		p.endPoints.Remove(p.endPoints.Back())
	}

	if p.endPoints.Len() < 3 {
		return
	}

	gTriangleDrawer.activate()

	pa := make(p2t.PointArray, 0, p.endPoints.Len())
	for e := p.endPoints.Front(); e != nil; e = e.Next() {
		x, y := XY(e.Value.(image.Point))
		pa = append(pa, &p2t.Point{X: float64(x), Y: float64(y)})
	}
	p2t.Init(pa)
	triArr := p2t.Triangulate()
	vertices := make([]float32, 6, 6)
	for _, tri := range triArr {
		for i, triPt := range tri.Point {
			pt := canv.toGLPoint(image.Pt(int(triPt.X), int(triPt.Y)))
			vertices[i*2] = pt.X
			vertices[i*2+1] = pt.Y
		}
		gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, vertices, gl.STATIC_DRAW)
		gl.DrawArrays(gl.TRIANGLES, 0, 3)
	}
}

func MakeQuadraticCurve(p1, c, p2 image.Point) QuadraticCurve {
	return QuadraticCurve{
		Points: [3]image.Point{p1, c, p2},
	}
}

func ToPoint(v mathgl.Vec2f) image.Point {
	return image.Point{int(v[0]), int(v[1])}
}

func makeQuadraticCurve(p1, c, p2 mathgl.Vec2f) QuadraticCurve {
	return QuadraticCurve{
		Points: [3]image.Point{
			ToPoint(p1),
			ToPoint(c),
			ToPoint(p2),
		},
	}
}

func MakeBezierCurve(p1, c1, c2, p2 image.Point) BezierCurve {
	return BezierCurve{
		Points: [4]image.Point{p1, c1, c2, p2},
	}
}

type GLPoint struct {
	X, Y float32
}

func (canv Canvas) toGLPoint(p image.Point) GLPoint {
	return GLPoint{float32(p.X) / float32(canv.W), float32(p.Y) / float32(canv.H)}
}

func (canv Canvas) toGLPoints(points []image.Point) []GLPoint {
	ps := make([]GLPoint, len(points))
	for i, p := range points {
		ps[i] = canv.toGLPoint(p)
	}
	return ps
}

func (c QuadraticCurve) draw(canv Canvas) {
	p := canv.toGLPoints(c.Points[:])
	vertices := []float32{
		p[0].X, p[0].Y, 0.0, 0.0,
		p[1].X, p[1].Y, 0.5, 0.0,
		p[2].X, p[2].Y, 1.0, 1.0,
	}
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, vertices, gl.STATIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
}

func (c QuadraticCurve) Draw(canv Canvas) {
	gQuadraticDrawer.activate()
	c.draw(canv)
}

func Vectorf(p image.Point) (v mathgl.Vec2f) {
	v[0], v[1] = float32(p.X), float32(p.Y)
	return v
}

func (c BezierCurve) quadApprox(p1, c1, c2, p2 mathgl.Vec2f) (v mathgl.Vec2f, ok bool) {
	//P2 - 3·C2 + 3·C1 - P1
	d01 := p2.Sub(c2.Mul(3)).Add(c1.Mul(3)).Sub(p1).Len() / 2
	if d01 <= gQuadraticApproxPrecision {
		// (3·C2 - P2 + 3·C1 - P1)/4
		return c2.Mul(3).Sub(p2).Add(c1.Mul(3)).Sub(p1).Mul(1 / 4.), true
	}
	return v, false
}

func mid(v1 mathgl.Vec2f, v2 mathgl.Vec2f) mathgl.Vec2f {
	return v1.Add(v2).Mul(1 / 2.)
}

func (c BezierCurve) toQuadratics(p1, c1, c2, p2 mathgl.Vec2f) []QuadraticCurve {
	if newcp, ok := c.quadApprox(p1, c1, c2, p2); ok {
		return []QuadraticCurve{makeQuadraticCurve(p1, newcp, p2)}
	}

	p4, p6 := mid(p1, c1), mid(p2, c2)
	p5 := mid(c1, c2)
	p7, p8 := mid(p4, p5), mid(p5, p6)
	p9 := mid(p7, p8)

	return append(c.toQuadratics(p1, p4, p7, p9), c.toQuadratics(p9, p8, p6, p2)...)
}

//ToQuadratics approximates a cubic bezier curve with quadratics.
//Algorithm by Adrian Colomitchi at
//http://www.caffeineowl.com/graphics/2d/vectorial/cubic2quad01.html
func (c BezierCurve) ToQuadratics() []QuadraticCurve {
	p1, c1 := Vectorf(c.Points[0]), Vectorf(c.Points[1])
	c2, p2 := Vectorf(c.Points[2]), Vectorf(c.Points[3])
	return c.toQuadratics(p1, c1, c2, p2)
}

func (c BezierCurve) Draw(canv Canvas) {
	gQuadraticDrawer.activate()
	quads := c.ToQuadratics()
	if len(quads) < 1 {
		panic("Something's wrong.")
	}
	path := NewPath().StartAt(quads[0].Points[0])
	for _, quadc := range quads {
		path.QuadraticTo(quadc.Points[2], quadc.Points[1])
	}
	path.draw(canv)
}

func ShaderFromFile(stype gl.GLenum, filename string) (shader glh.Shader) {
	_, f, _, _ := runtime.Caller(0)
	dir := path.Dir(f)
	fcont, _ := ioutil.ReadFile(path.Join(dir, filename))
	shader = glh.Shader{stype, string(fcont[:])}
	shader.Compile()
	return shader
}
