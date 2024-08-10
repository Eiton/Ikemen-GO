//go:build !kinc

package main

import (
	"bytes"
	_ "embed" // Support for go:embed resources
	"encoding/binary"
	"fmt"
	"runtime"
	"strconv"
	"unsafe"

	gl "github.com/go-gl/gl/v2.1/gl"
	glfw "github.com/go-gl/glfw/v3.3/glfw"
	"golang.org/x/mobile/exp/f32"
)

var InternalFormatLUT = map[int32]uint32{
	8:  gl.LUMINANCE,
	24: gl.RGB,
	32: gl.RGBA,
}

var BlendEquationLUT = map[BlendEquation]uint32{
	BlendAdd:             gl.FUNC_ADD,
	BlendReverseSubtract: gl.FUNC_REVERSE_SUBTRACT,
}

var BlendFunctionLUT = map[BlendFunc]uint32{
	BlendOne:              gl.ONE,
	BlendZero:             gl.ZERO,
	BlendSrcAlpha:         gl.SRC_ALPHA,
	BlendOneMinusSrcAlpha: gl.ONE_MINUS_SRC_ALPHA,
}

var PrimitiveModeLUT = map[PrimitiveMode]uint32{
	LINES:          gl.LINES,
	LINE_LOOP:      gl.LINE_LOOP,
	LINE_STRIP:     gl.LINE_STRIP,
	TRIANGLES:      gl.TRIANGLES,
	TRIANGLE_STRIP: gl.TRIANGLE_STRIP,
	TRIANGLE_FAN:   gl.TRIANGLE_FAN,
}

// ------------------------------------------------------------------
// Util
func glStr(s string) *uint8 {
	return gl.Str(s + "\x00")
}

// ------------------------------------------------------------------
// ShaderProgram

type ShaderProgram struct {
	// Program
	program uint32
	// Attributes
	a map[string]int32
	// Uniforms
	u map[string]int32
	// Texture units
	t map[string]int
}

func newShaderProgram(vert, frag, geo, id string) (s *ShaderProgram) {
	vertObj := compileShader(gl.VERTEX_SHADER, vert)
	fragObj := compileShader(gl.FRAGMENT_SHADER, frag)
	var prog uint32
	if len(geo) > 0 {
		geoObj := compileShader(gl.GEOMETRY_SHADER_EXT, geo)
		prog = linkProgram(vertObj, fragObj, geoObj)
	} else {
		prog = linkProgram(vertObj, fragObj)
	}

	s = &ShaderProgram{program: prog}
	s.a = make(map[string]int32)
	s.u = make(map[string]int32)
	s.t = make(map[string]int)
	return
}
func (s *ShaderProgram) RegisterAttributes(names ...string) {
	for _, name := range names {
		s.a[name] = gl.GetAttribLocation(s.program, glStr(name))
	}
}

func (s *ShaderProgram) RegisterUniforms(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, glStr(name))
	}
}

func (s *ShaderProgram) RegisterTextures(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, glStr(name))
		s.t[name] = len(s.t)
	}
}

func compileShader(shaderType uint32, src string) (shader uint32) {
	shader = gl.CreateShader(shaderType)
	src = "#version 120\n" + src + "\x00"
	s, _ := gl.Strs(src)
	var l int32 = int32(len(src) - 1)
	gl.ShaderSource(shader, 1, s, &l)
	gl.CompileShader(shader)
	var ok int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &ok)
	if ok == 0 {
		var err error
		var size, l int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &size)
		if size > 0 {
			str := make([]byte, size+1)
			gl.GetShaderInfoLog(shader, size, &l, &str[0])
			err = Error(str[:l])
		}
		chk(err)
		gl.DeleteShader(shader)
		panic(Error("Shader compile error"))
	}
	return
}

func linkProgram(params ...uint32) (program uint32) {
	program = gl.CreateProgram()
	for _, param := range params {
		gl.AttachShader(program, param)
	}
	if len(params) > 2 {
		// Geometry Shader Params
		gl.ProgramParameteriEXT(program, gl.GEOMETRY_INPUT_TYPE_EXT, gl.TRIANGLES)
		gl.ProgramParameteriEXT(program, gl.GEOMETRY_OUTPUT_TYPE_EXT, gl.TRIANGLE_STRIP)
		gl.ProgramParameteriEXT(program, gl.GEOMETRY_VERTICES_OUT_EXT, 3*6)
	}
	gl.LinkProgram(program)
	// Mark shaders for deletion when the program is deleted
	for _, param := range params {
		gl.DeleteShader(param)
	}
	var ok int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &ok)
	if ok == 0 {
		var err error
		var size, l int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &size)
		if size > 0 {
			str := make([]byte, size+1)
			gl.GetProgramInfoLog(program, size, &l, &str[0])
			err = Error(str[:l])
		}
		chk(err)
		gl.DeleteProgram(program)
		panic(Error("Link error"))
	}
	return
}

// ------------------------------------------------------------------
// Texture

type Texture struct {
	width  int32
	height int32
	depth  int32
	filter bool
	handle uint32
}

// Generate a new texture name
func newTexture(width, height, depth int32, filter bool) (t *Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture{width, height, depth, filter, h}
	runtime.SetFinalizer(t, func(t *Texture) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	return
}

func newDataTexture(width, height int32) (t *Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture{width, height, 32, false, h}
	runtime.SetFinalizer(t, func(t *Texture) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	//gl.TexImage2D(gl.TEXTURE_2D, 0, 32, t.width, t.height, 0, 36, gl.FLOAT, unsafe.Pointer(&data[0]))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	return
}

// Bind a texture and upload texel data to it
func (t *Texture) SetData(data []byte) {
	var interp int32 = gl.NEAREST
	if t.filter {
		interp = gl.LINEAR
	}

	format := InternalFormatLUT[Max(t.depth, 8)]

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	if data != nil {
		gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	} else {
		gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, nil)
	}

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
}
func (t *Texture) SetDataG(data []byte, mag, min, ws, wt int32) {

	format := InternalFormatLUT[Max(t.depth, 8)]

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	gl.GenerateMipmap(gl.TEXTURE_2D)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, mag)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, min)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, ws)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, wt)
}
func (t *Texture) SetPixelData(data []float32) {

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F_ARB, t.width, t.height, 0, gl.RGBA, gl.FLOAT, unsafe.Pointer(&data[0]))
}

// Return whether texture has a valid handle
func (t *Texture) IsValid() bool {
	return t.handle != 0
}

// ------------------------------------------------------------------
// Renderer

type Renderer struct {
	fbo         uint32
	fbo_texture uint32
	// Normal rendering
	rbo_depth uint32
	// MSAA rendering
	fbo_f         uint32
	fbo_f_texture *Texture
	// Shadow Map
	fbo_shadow              uint32
	fbo_shadow_texture      [8]uint32
	fbo_shadow_cube_texture [8]uint32
	// Post-processing shaders
	postVertBuffer   uint32
	postShaderSelect []*ShaderProgram
	// Shader and vertex data for primitive rendering
	spriteShader *ShaderProgram
	vertexBuffer uint32
	// Shader and index data for 3D model rendering
	shadowMapShader   *ShaderProgram
	modelShader       *ShaderProgram
	stageVertexBuffer uint32
	stageIndexBuffer  uint32
}

//go:embed shaders/sprite.vert.glsl
var vertShader string

//go:embed shaders/sprite.frag.glsl
var fragShader string

//go:embed shaders/model.vert.glsl
var modelVertShader string

//go:embed shaders/model.frag.glsl
var modelFragShader string

//go:embed shaders/shadow.vert.glsl
var shadowVertShader string

//go:embed shaders/shadow.frag.glsl
var shadowFragShader string

//go:embed shaders/shadow.geo.glsl
var shadowGeoShader string

//go:embed shaders/ident.vert.glsl
var identVertShader string

//go:embed shaders/ident.frag.glsl
var identFragShader string

// Render initialization.
// Creates the default shaders, the framebuffer and enables MSAA.
func (r *Renderer) Init() {
	chk(gl.Init())
	sys.errLog.Printf("Using OpenGL %v (%v)", gl.GetString(gl.VERSION), gl.GetString(gl.RENDERER))

	// Store current timestamp
	sys.prevTimestamp = glfw.GetTime()

	r.postShaderSelect = make([]*ShaderProgram, 1+len(sys.externalShaderList))

	// Data buffers for rendering
	postVertData := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)

	gl.GenBuffers(1, &r.postVertBuffer)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(postVertData), unsafe.Pointer(&postVertData[0]), gl.STATIC_DRAW)

	gl.GenBuffers(1, &r.vertexBuffer)
	gl.GenBuffers(1, &r.stageVertexBuffer)
	gl.GenBuffers(1, &r.stageIndexBuffer)

	// Sprite shader
	r.spriteShader = newShaderProgram(vertShader, fragShader, "", "Main Shader")
	r.spriteShader.RegisterAttributes("position", "uv")
	r.spriteShader.RegisterUniforms("modelview", "projection", "x1x2x4x3",
		"alpha", "tint", "mask", "neg", "gray", "add", "mult", "isFlat", "isRgba", "isTrapez", "hue")
	r.spriteShader.RegisterTextures("pal", "tex")

	// 3D model shader
	r.modelShader = newShaderProgram(modelVertShader, modelFragShader, "", "Model Shader")
	r.modelShader.RegisterAttributes("vertexId", "position", "uv", "normalIn", "tangentIn", "vertColor", "joints_0", "joints_1", "weights_0", "weights_1")
	r.modelShader.RegisterUniforms("model", "view", "projection", "farPlane", "normalMatrix", "unlit", "baseColorFactor", "add", "mult", "useTexture", "useNormalMap", "useMetallicRoughnessMap", "neg", "gray", "hue",
		"enableAlpha", "alphaThreshold", "numJoints", "morphTargetWeight", "morphTargetOffset", "numTargets", "numVertices",
		"metallicRoughness", "ambientOcclusion",
		"cameraPosition",
		"lightMatrices[0]", "lightMatrices[1]", "lightMatrices[2]", "lightMatrices[3]", "lightMatrices[4]", "lightMatrices[5]", "lightMatrices[6]", "lightMatrices[7]",
		"lights[0].direction", "lights[0].range", "lights[0].color", "lights[0].intensity", "lights[0].position", "lights[0].innerConeCos", "lights[0].outerConeCos", "lights[0].type",
		"lights[1].direction", "lights[1].range", "lights[1].color", "lights[1].intensity", "lights[1].position", "lights[1].innerConeCos", "lights[1].outerConeCos", "lights[1].type",
		"lights[2].direction", "lights[2].range", "lights[2].color", "lights[2].intensity", "lights[2].position", "lights[2].innerConeCos", "lights[2].outerConeCos", "lights[2].type",
		"lights[3].direction", "lights[3].range", "lights[3].color", "lights[3].intensity", "lights[3].position", "lights[3].innerConeCos", "lights[3].outerConeCos", "lights[3].type",
		"lights[4].direction", "lights[4].range", "lights[4].color", "lights[4].intensity", "lights[4].position", "lights[4].innerConeCos", "lights[4].outerConeCos", "lights[4].type",
		"lights[5].direction", "lights[5].range", "lights[5].color", "lights[5].intensity", "lights[5].position", "lights[5].innerConeCos", "lights[5].outerConeCos", "lights[5].type",
		"lights[6].direction", "lights[6].range", "lights[6].color", "lights[6].intensity", "lights[6].position", "lights[6].innerConeCos", "lights[6].outerConeCos", "lights[6].type",
		"lights[7].direction", "lights[7].range", "lights[7].color", "lights[7].intensity", "lights[7].position", "lights[7].innerConeCos", "lights[7].outerConeCos", "lights[7].type",
	)
	r.modelShader.RegisterTextures("tex", "morphTargetValues", "jointMatrices", "normalMap", "metallicRoughnessMap", "ambientOcclusionMap",
		"shadowMap[0]", "shadowMap[1]", "shadowMap[2]", "shadowMap[3]", "shadowMap[4]", "shadowMap[5]", "shadowMap[6]", "shadowMap[7]",
		"shadowCubeMap[0]", "shadowCubeMap[1]", "shadowCubeMap[2]", "shadowCubeMap[3]", "shadowCubeMap[4]", "shadowCubeMap[5]", "shadowCubeMap[6]", "shadowCubeMap[7]")

	r.shadowMapShader = newShaderProgram(shadowVertShader, shadowFragShader, shadowGeoShader, "Shadow Map Shader")
	//r.shadowMapShader = newShaderProgram(shadowVertShader, shadowFragShader, "", "Shadow Map Shader")
	r.shadowMapShader.RegisterAttributes("vertexId", "position", "vertColor", "uv", "joints_0", "joints_1", "weights_0", "weights_1")
	r.shadowMapShader.RegisterUniforms("model", "lightMatrices[0]", "lightMatrices[1]", "lightMatrices[2]", "lightMatrices[3]", "lightMatrices[4]", "lightMatrices[5]", "isPointLight", "lightPos", "farPlane", "numJoints", "morphTargetWeight", "morphTargetOffset", "numTargets", "numVertices", "enableAlpha", "alphaThreshold", "baseColorFactor", "useTexture")
	r.shadowMapShader.RegisterTextures("morphTargetValues", "jointMatrices", "tex")

	// Compile postprocessing shaders

	// Calculate total amount of shaders loaded.
	r.postShaderSelect = make([]*ShaderProgram, 1+len(sys.externalShaderList))

	// Ident shader (no postprocessing)
	r.postShaderSelect[0] = newShaderProgram(identVertShader, identFragShader, "", "Identity Postprocess")
	r.postShaderSelect[0].RegisterAttributes("VertCoord")
	r.postShaderSelect[0].RegisterUniforms("Texture", "TextureSize")

	// External Shaders
	for i := 0; i < len(sys.externalShaderList); i++ {
		r.postShaderSelect[1+i] = newShaderProgram(sys.externalShaders[0][i],
			sys.externalShaders[1][i], "", fmt.Sprintf("Postprocess Shader #%v", i+1))
		r.postShaderSelect[1+i].RegisterUniforms("Texture", "TextureSize")
	}

	if sys.multisampleAntialiasing > 0 {
		gl.Enable(gl.MULTISAMPLE)
	}

	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &r.fbo_texture)

	if sys.multisampleAntialiasing > 0 {
		gl.BindTexture(gl.TEXTURE_2D_MULTISAMPLE, r.fbo_texture)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_texture)
	}

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	if sys.multisampleAntialiasing > 0 {
		gl.TexImage2DMultisample(gl.TEXTURE_2D_MULTISAMPLE, sys.multisampleAntialiasing, gl.RGBA, sys.scrrect[2], sys.scrrect[3], true)

	} else {
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, sys.scrrect[2], sys.scrrect[3], 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	}

	gl.BindTexture(gl.TEXTURE_2D, 0)

	//r.rbo_depth = gl.CreateRenderbuffer()
	gl.GenRenderbuffers(1, &r.rbo_depth)

	gl.BindRenderbuffer(gl.RENDERBUFFER, r.rbo_depth)
	if sys.multisampleAntialiasing > 0 {
		//gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, int(sys.scrrect[2]), int(sys.scrrect[3]))
		gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, sys.multisampleAntialiasing, gl.DEPTH_COMPONENT16, sys.scrrect[2], sys.scrrect[3])
	} else {
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, sys.scrrect[2], sys.scrrect[3])
	}
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)
	if sys.multisampleAntialiasing > 0 {
		r.fbo_f_texture = newTexture(sys.scrrect[2], sys.scrrect[3], 32, false)
		r.fbo_f_texture.SetData(nil)
	} else {
		//r.rbo_depth = gl.CreateRenderbuffer()
		//gl.BindRenderbuffer(gl.RENDERBUFFER, r.rbo_depth)
		//gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, int(sys.scrrect[2]), int(sys.scrrect[3]))
		//gl.BindRenderbuffer(gl.RENDERBUFFER, gl.NoRenderbuffer)
	}

	gl.GenFramebuffers(1, &r.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)

	if sys.multisampleAntialiasing > 0 {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D_MULTISAMPLE, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
		if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
			sys.errLog.Printf("framebuffer create failed: 0x%x", status)
			fmt.Printf("framebuffer create failed: 0x%x \n", status)
		}
		gl.GenFramebuffers(1, &r.fbo_f)
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_f)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_f_texture.handle, 0)
	} else {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
	}
	gl.GenFramebuffersEXT(1, &r.fbo_shadow)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(8, &r.fbo_shadow_texture[0])
	gl.GenTextures(8, &r.fbo_shadow_cube_texture[0])

	for i := 0; i < 8; i++ {
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_shadow_texture[i])

		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.DEPTH_COMPONENT, 1024, 1024, 0, gl.DEPTH_COMPONENT, gl.FLOAT, nil)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

		gl.BindTexture(gl.TEXTURE_CUBE_MAP, r.fbo_shadow_cube_texture[i])
		for j := 0; j < 6; j++ {
			gl.TexImage2D(uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+j), 0, gl.DEPTH_COMPONENT, 1024, 1024, 0, gl.DEPTH_COMPONENT, gl.FLOAT, nil)
		}
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	}
	gl.BindFramebufferEXT(gl.FRAMEBUFFER, r.fbo_shadow)
	//gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, r.fbo_shadow_texture[0], 0)
	gl.DrawBuffer(gl.NONE)
	gl.ReadBuffer(gl.NONE)
	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		sys.errLog.Printf("framebuffer create failed: 0x%x", status)
	}

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (r *Renderer) Close() {
}

func (r *Renderer) BeginFrame(clearColor bool) {
	sys.absTickCountF++
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	if clearColor {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	} else {
		gl.Clear(gl.DEPTH_BUFFER_BIT)
	}
}

func (r *Renderer) EndFrame() {
	if sys.multisampleAntialiasing > 0 {
		gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, r.fbo_f)
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo)
		gl.BlitFramebuffer(0, 0, sys.scrrect[2], sys.scrrect[3], 0, 0, sys.scrrect[2], sys.scrrect[3], gl.COLOR_BUFFER_BIT, gl.LINEAR)
	}

	x, y, resizedWidth, resizedHeight := sys.window.GetScaledViewportSize()
	postShader := r.postShaderSelect[sys.postProcessingShader]

	var scaleMode uint32 // GL enum
	if sys.windowScaleMode == true {
		scaleMode = gl.LINEAR
	} else {
		scaleMode = gl.NEAREST
	}

	gl.UseProgram(postShader.program)
	gl.Disable(gl.BLEND)

	gl.ActiveTexture(gl.TEXTURE0)
	if sys.multisampleAntialiasing > 0 {
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_f_texture.handle)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_texture)
	}
	gl.Uniform1i(postShader.u["Texture"], 0)
	gl.Uniform2f(postShader.u["TextureSize"], float32(sys.scrrect[2]), float32(sys.scrrect[3]))

	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)
	gl.Finish()

	loc := r.modelShader.a["VertCoord"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, 0)

	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.DisableVertexAttribArray(uint32(loc))

	// rebind to prepare frame for blitting to window
	if sys.multisampleAntialiasing > 0 {
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo_f)
	} else {
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo)
	}
	gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)

	// clear the entire window's contents (prevents garbage data artifacts when resizing)
	fullw, fullh := sys.window.GetSize()
	gl.Viewport(0, 0, int32(fullw), int32(fullh))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// scale finished frame to window
	gl.BlitFramebuffer(0, 0, sys.scrrect[2], sys.scrrect[3], x, y, x+resizedWidth, y+resizedHeight, gl.COLOR_BUFFER_BIT, scaleMode)
}

func (r *Renderer) SetPipeline(eq BlendEquation, src, dst BlendFunc) {
	gl.UseProgram(r.spriteShader.program)

	gl.BlendEquation(BlendEquationLUT[eq])
	gl.BlendFunc(BlendFunctionLUT[src], BlendFunctionLUT[dst])
	gl.Enable(gl.BLEND)

	// Must bind buffer before enabling attributes
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	loc := r.spriteShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 0)
	loc = r.spriteShader.a["uv"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 8)
}

func (r *Renderer) ReleasePipeline() {
	loc := r.spriteShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.spriteShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.Disable(gl.BLEND)
}

func (r *Renderer) prepareShadowMapPipeline() {
	gl.UseProgram(r.shadowMapShader.program)
	gl.BindFramebufferEXT(gl.FRAMEBUFFER, r.fbo_shadow)
	gl.Viewport(0, 0, 1024, 1024)
	gl.Enable(gl.TEXTURE_2D)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
	//gl.DepthFunc(gl.LESS)
	//gl.DepthMask(true)

	gl.BlendEquation(gl.FUNC_ADD)
	gl.BlendFunc(gl.ONE, gl.ZERO)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
}
func (r *Renderer) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	if invertFrontFace {
		gl.FrontFace(gl.CW)
	} else {
		gl.FrontFace(gl.CCW)
	}
	if !doubleSided {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	} else {
		gl.Disable(gl.CULL_FACE)
	}

	loc := r.shadowMapShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.shadowMapShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		loc = r.shadowMapShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else {
		loc = r.shadowMapShader.a["uv"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}
	if useNormal {
		offset += 12 * numVertices
	}
	if useTangent {
		offset += 16 * numVertices
	}
	if useVertColor {
		loc = r.shadowMapShader.a["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.shadowMapShader.a["vertColor"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		loc = r.shadowMapShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.shadowMapShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			loc = r.shadowMapShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.shadowMapShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else {
			loc = r.shadowMapShader.a["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.shadowMapShader.a["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else {
		loc = r.shadowMapShader.a["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}

func (r *Renderer) ReleaseShadowPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.BLEND)
}
func (r *Renderer) prepareModelPipeline() {
	gl.UseProgram(r.modelShader.program)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.TEXTURE_CUBE_MAP)
	gl.Enable(gl.BLEND)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
	for i := 0; i < 8; i++ {
		loc, unit := r.modelShader.u["shadowMap["+strconv.Itoa(i)+"]"], r.modelShader.t["shadowMap["+strconv.Itoa(i)+"]"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_shadow_texture[i])
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["shadowCubeMap["+strconv.Itoa(i)+"]"], r.modelShader.t["shadowCubeMap["+strconv.Itoa(i)+"]"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, r.fbo_shadow_cube_texture[i])
		gl.Uniform1i(loc, int32(unit))
	}

}
func (r *Renderer) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	if depthTest {
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(gl.LESS)
	} else {
		gl.Disable(gl.DEPTH_TEST)
	}
	gl.DepthMask(depthMask)
	if invertFrontFace {
		gl.FrontFace(gl.CW)
	} else {
		gl.FrontFace(gl.CCW)
	}
	if !doubleSided {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	} else {
		gl.Disable(gl.CULL_FACE)
	}

	gl.BlendEquation(BlendEquationLUT[eq])
	gl.BlendFunc(BlendFunctionLUT[src], BlendFunctionLUT[dst])

	loc := r.modelShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.modelShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		loc = r.modelShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else {
		loc = r.modelShader.a["uv"]
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}
	if useNormal {
		loc = r.modelShader.a["normalIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
		offset += 12 * numVertices
	} else {
		loc = r.modelShader.a["normalIn"]
		gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	}
	if useTangent {
		loc = r.modelShader.a["tangentIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.modelShader.a["tangentIn"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useVertColor {
		loc = r.modelShader.a["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.modelShader.a["vertColor"]
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		loc = r.modelShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.modelShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			loc = r.modelShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.modelShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else {
			loc = r.modelShader.a["joints_1"]
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.modelShader.a["weights_1"]
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else {
		loc = r.modelShader.a["joints_0"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_0"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["joints_1"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_1"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}
func (r *Renderer) ReleaseModelPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
}

func (r *Renderer) ReadPixels(data []uint8, width, height int) {
	r.EndFrame()
	sys.window.SwapBuffers()
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	r.BeginFrame(false)
}

func (r *Renderer) Scissor(x, y, width, height int32) {
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(x, sys.scrrect[3]-(y+height), width, height)
}

func (r *Renderer) DisableScissor() {
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *Renderer) SetUniformI(name string, val int) {
	loc := r.spriteShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer) SetUniformF(name string, values ...float32) {
	loc := r.spriteShader.u[name]
	switch len(values) {
	case 1:
		gl.Uniform1f(loc, values[0])
	case 2:
		gl.Uniform2f(loc, values[0], values[1])
	case 3:
		gl.Uniform3f(loc, values[0], values[1], values[2])
	case 4:
		gl.Uniform4f(loc, values[0], values[1], values[2], values[3])
	}
}

func (r *Renderer) SetUniformFv(name string, values []float32) {
	loc := r.spriteShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	}
}

func (r *Renderer) SetUniformMatrix(name string, value []float32) {
	loc := r.spriteShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer) SetTexture(name string, t *Texture) {
	loc, unit := r.spriteShader.u[name], r.spriteShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer) SetModelUniformI(name string, val int) {
	loc := r.modelShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer) SetModelUniformF(name string, values ...float32) {
	loc := r.modelShader.u[name]
	switch len(values) {
	case 1:
		gl.Uniform1f(loc, values[0])
	case 2:
		gl.Uniform2f(loc, values[0], values[1])
	case 3:
		gl.Uniform3f(loc, values[0], values[1], values[2])
	case 4:
		gl.Uniform4f(loc, values[0], values[1], values[2], values[3])
	}
}
func (r *Renderer) SetModelUniformFv(name string, values []float32) {
	loc := r.modelShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	case 8:
		gl.Uniform4fv(loc, 2, &values[0])
	}
}
func (r *Renderer) SetModelUniformMatrix(name string, value []float32) {
	loc := r.modelShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer) SetModelTexture(name string, t *Texture) {
	loc, unit := r.modelShader.u[name], r.modelShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer) SetShadowMapUniformI(name string, val int) {
	loc := r.shadowMapShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer) SetShadowMapUniformF(name string, values ...float32) {
	loc := r.shadowMapShader.u[name]
	switch len(values) {
	case 1:
		gl.Uniform1f(loc, values[0])
	case 2:
		gl.Uniform2f(loc, values[0], values[1])
	case 3:
		gl.Uniform3f(loc, values[0], values[1], values[2])
	case 4:
		gl.Uniform4f(loc, values[0], values[1], values[2], values[3])
	}
}
func (r *Renderer) SetShadowMapUniformFv(name string, values []float32) {
	loc := r.shadowMapShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	case 8:
		gl.Uniform4fv(loc, 2, &values[0])
	}
}
func (r *Renderer) SetShadowMapUniformMatrix(name string, value []float32) {
	loc := r.shadowMapShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer) SetShadowMapTexture(name string, t *Texture) {
	loc, unit := r.shadowMapShader.u[name], r.shadowMapShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer) SetShadowFrameTexture(i uint32) {
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, r.fbo_shadow_texture[i], 0)
	gl.Clear(gl.DEPTH_BUFFER_BIT)
}

func (r *Renderer) SetShadowFrameCubeTexture(i uint32) {
	gl.FramebufferTextureEXT(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT_EXT, r.fbo_shadow_cube_texture[i], 0)
	gl.Clear(gl.DEPTH_BUFFER_BIT)
}

func (r *Renderer) SetVertexData(values ...float32) {
	data := f32.Bytes(binary.LittleEndian, values...)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}
func (r *Renderer) SetStageVertexData(values []byte) {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(values), unsafe.Pointer(&values[0]), gl.STATIC_DRAW)
}
func (r *Renderer) SetStageIndexData(values ...uint32) {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, values)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(values)*4, unsafe.Pointer(&data.Bytes()[0]), gl.STATIC_DRAW)
}

func (r *Renderer) RenderQuad() {
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
}
func (r *Renderer) RenderElements(mode PrimitiveMode, count, offset int) {
	gl.DrawElementsWithOffset(PrimitiveModeLUT[mode], int32(count), gl.UNSIGNED_INT, uintptr(offset))
}
