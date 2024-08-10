#extension GL_EXT_geometry_shader4: enable

uniform bool isPointLight;

varying in vec4 vColor[3];
varying in vec2 texcoord[3];
varying out vec4 FragPos;
varying out vec4 vColor0;
varying out vec2 texcoord0;
uniform mat4 lightMatrices[6];
void main() {
    if(isPointLight){
        for(int face = 0; face < 6; ++face)
        {
            gl_Layer = face; // built-in variable that specifies to which face we render.
            for(int i = 0; i < 3; ++i) // for each triangle vertex
            {
                FragPos = gl_PositionIn[i];
                texcoord0 = texcoord[i];
                vColor0 = vColor[i];
                gl_Position = lightMatrices[face] * gl_PositionIn[i];
                EmitVertex();
            }    
            EndPrimitive();
        }
    }else{
        gl_Layer = 0;
        for(int i = 0; i < 3; ++i) // for each triangle vertex
        {
            FragPos = gl_PositionIn[i];
            texcoord0 = texcoord[i];
            vColor0 = vColor[i];
            gl_Position = lightMatrices[0] * gl_PositionIn[i];
            EmitVertex();
        }
        EndPrimitive();
    }
} 