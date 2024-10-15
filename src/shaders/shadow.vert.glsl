#extension GL_EXT_gpu_shader4 : enable
uniform mat4 model;
uniform sampler2D jointMatrices;
//uniform highp sampler2D morphTargetValues;
uniform sampler2D morphTargetValues;
uniform int numJoints;
uniform int numTargets;
uniform vec4 morphTargetWeight[2];
uniform vec4 morphTargetOffset;
uniform int numVertices;

//gl_VertexID is not available in 1.2
attribute float vertexId;
attribute vec3 position;
attribute vec4 vertColor;
attribute vec2 uv;
attribute vec4 joints_0;
attribute vec4 joints_1;
attribute vec4 weights_0;
attribute vec4 weights_1;
varying vec4 vColor;
varying vec2 texcoord;


mat4 getMatrixFromTexture(float index){
	mat4 mat;
	mat[0] = texture2D(jointMatrices,vec2(0.5/6.0,(index+0.5)/numJoints));
	mat[1] = texture2D(jointMatrices,vec2(1.5/6.0,(index+0.5)/numJoints));
	mat[2] = texture2D(jointMatrices,vec2(2.5/6.0,(index+0.5)/numJoints));
	mat[3] = vec4(0,0,0,1);
	return transpose(mat);
}
mat4 getJointMatrix(){
	mat4 ret = mat4(0);
	ret += weights_0.x*getMatrixFromTexture(joints_0.x);
	ret += weights_0.y*getMatrixFromTexture(joints_0.y);
	ret += weights_0.z*getMatrixFromTexture(joints_0.z);
	ret += weights_0.w*getMatrixFromTexture(joints_0.w);
	ret += weights_1.x*getMatrixFromTexture(joints_1.x);
	ret += weights_1.y*getMatrixFromTexture(joints_1.y);
	ret += weights_1.z*getMatrixFromTexture(joints_1.z);
	ret += weights_1.w*getMatrixFromTexture(joints_1.w);
	if(ret == mat4(0.0)){
		return mat4(1.0);
	}
	return ret;
}
void main(void) {
	texcoord = uv;
	vColor = vertColor;
	vec4 pos = vec4(position, 1.0);
	if(morphTargetOffset[0] > 0){
		for(int idx = 0; idx < numTargets; ++idx)
		{
			if(idx < morphTargetOffset[0]){
				pos += morphTargetWeight[idx/4][idx%4] * texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8));
			}else if(idx >= morphTargetOffset[2] && idx < morphTargetOffset[3]){
				texcoord += morphTargetWeight[idx/4][idx%4] * vec2(texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8)));
			}
		}
	}
	if(weights_0.x+weights_0.y+weights_0.z+weights_0.w+weights_1.x+weights_1.y+weights_1.z+weights_1.w > 0){
		
		mat4 jointMatrix = getJointMatrix();
		gl_Position = model * jointMatrix * pos;
	}else{
		gl_Position = model * pos;
	}
}