#if __VERSION__ >= 130
#define COMPAT_POS_IN(i) gl_in[i].gl_Position
#define LIGHT_COUNT 4

layout(triangles) in;
layout(triangle_strip, max_vertices = 72) out;
in vec4 vColor[];
in vec2 texcoord[];
out vec4 FragPos;
out float vColorAlpha;
out vec2 texcoord0;
flat out int lightIndex;
#else
#extension GL_EXT_geometry_shader4: enable
#define COMPAT_POS_IN(i) gl_PositionIn[i]
#define LIGHT_COUNT 1

varying in vec4 vColor[3];
varying in vec2 texcoord[3];
varying out vec4 FragPos;
varying out float vColorAlpha;
varying out vec2 texcoord0;
varying flat out int lightIndex;
#endif

uniform int lightType[LIGHT_COUNT];

uniform mat4 lightMatrices[24];

const int LightType_None = 0;
const int LightType_Directional = 1;
const int LightType_Point = 2;
const int LightType_Spot = 3;
void main() {
    for(int light = 0;light < LIGHT_COUNT;light++){
        lightIndex = light;
        if(lightType[light] == LightType_Point){
            for(int face = 0; face < 6; ++face)
            {
                gl_Layer = light*6+face; // built-in variable that specifies to which face we render.
                for(int i = 0; i < 3; ++i) // for each triangle vertex
                {
                    FragPos = COMPAT_POS_IN(i);
                    texcoord0 = texcoord[i];
                    vColorAlpha = vColor[i].a;
                    gl_Position = lightMatrices[light*6+face] * COMPAT_POS_IN(i);
                    EmitVertex();
                }    
                EndPrimitive();
            }
        }else if(lightType[light] != LightType_None){
            gl_Layer = light*6;
            for(int i = 0; i < 3; ++i) // for each triangle vertex
            {
                FragPos = COMPAT_POS_IN(i);
                texcoord0 = texcoord[i];
                vColorAlpha = vColor[i].a;
                gl_Position = lightMatrices[light*6] * COMPAT_POS_IN(i);
                EmitVertex();
            }
            EndPrimitive();
        }
    }
} 