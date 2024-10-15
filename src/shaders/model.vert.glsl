#extension GL_EXT_gpu_shader4 : enable
uniform mat4 model, view, projection;
uniform mat4 normalMatrix;
uniform mat4 lightMatrices[8];
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
attribute vec3 normalIn;
attribute vec4 tangentIn;
attribute vec2 uv;
attribute vec4 vertColor;
attribute vec4 joints_0;
attribute vec4 joints_1;
attribute vec4 weights_0;
attribute vec4 weights_1;
varying vec3 normal;
varying vec3 tangent;
varying vec3 bitangent;
varying vec2 texcoord;
varying vec4 vColor;
varying vec3 worldSpacePos;
varying vec4 lightSpacePos[8];


mat4 getMatrixFromTexture(float index){
	mat4 mat;
	mat[0] = texture2D(jointMatrices,vec2(0.5/6.0,(index+0.5)/numJoints));
	mat[1] = texture2D(jointMatrices,vec2(1.5/6.0,(index+0.5)/numJoints));
	mat[2] = texture2D(jointMatrices,vec2(2.5/6.0,(index+0.5)/numJoints));
	mat[3] = vec4(0,0,0,1);
	return transpose(mat);
}
mat4 getNormalMatrixFromTexture(float index){
	mat4 mat;
	mat[0] = texture2D(jointMatrices,vec2(3.5/6.0,(index+0.5)/numJoints));
	mat[1] = texture2D(jointMatrices,vec2(4.5/6.0,(index+0.5)/numJoints));
	mat[2] = texture2D(jointMatrices,vec2(5.5/6.0,(index+0.5)/numJoints));
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
mat3 getJointNormalMatrix(){
	mat4 ret = mat4(0);
	ret += weights_0.x*getNormalMatrixFromTexture(joints_0.x);
	ret += weights_0.y*getNormalMatrixFromTexture(joints_0.y);
	ret += weights_0.z*getNormalMatrixFromTexture(joints_0.z);
	ret += weights_0.w*getNormalMatrixFromTexture(joints_0.w);
	ret += weights_1.x*getNormalMatrixFromTexture(joints_1.x);
	ret += weights_1.y*getNormalMatrixFromTexture(joints_1.y);
	ret += weights_1.z*getNormalMatrixFromTexture(joints_1.z);
	ret += weights_1.w*getNormalMatrixFromTexture(joints_1.w);
	if(ret == mat4(0.0)){
		return mat3(1.0);
	}
	return mat3(ret);
}
void main(void) {
	texcoord = uv;
	vColor = vertColor;
	vec4 pos = vec4(position, 1.0);
	normal = normalIn;
	tangent = vec3(tangentIn);
	if(morphTargetWeight[0][0] != 0){
		for(int idx = 0; idx < numTargets; ++idx)
		{
			if(idx < morphTargetOffset[0]){
				pos += morphTargetWeight[idx/4][idx%4] * texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8));
			}else if(idx < morphTargetOffset[1]){
				normal += morphTargetWeight[idx/4][idx%4] * vec3(texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8)));
			}else if(idx < morphTargetOffset[2]){
				tangent += morphTargetWeight[idx/4][idx%4] * vec3(texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8)));
			}else if(idx < morphTargetOffset[3]){
				texcoord += morphTargetWeight[idx/4][idx%4] * vec2(texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8)));
			}else{
				vColor += morphTargetWeight[idx/4][idx%4] * texture2D(morphTargetValues,vec2((vertexId+0.5)/numVertices,(idx+0.5)/8));
			}
		}
	}
	if(weights_0.x+weights_0.y+weights_0.z+weights_0.w+weights_1.x+weights_1.y+weights_1.z+weights_1.w > 0){
		
		mat4 jointMatrix = getJointMatrix();
		mat3 jointNormalMatrix = getJointNormalMatrix();
		vec4 tmp2 = model * jointMatrix * pos;
		gl_Position = projection * view * tmp2;
		worldSpacePos = vec3(tmp2);
		for(int i = 0;i < 8;i++){
			lightSpacePos[i] = lightMatrices[i] * tmp2;
		}
		normal = mat3(normalMatrix) * jointNormalMatrix * normal;
	}else{
		vec4 tmp2 = model * pos;
		gl_Position = projection * view * tmp2;
		worldSpacePos = vec3(tmp2);
		for(int i = 0;i < 8;i++){
			lightSpacePos[i] = lightMatrices[i] * tmp2;
		}
		if(normal.x+normal.y+normal.z != 0){
			normal = normalize(mat3(normalMatrix) * normal);
			if(tangent.x+tangent.y+tangent.z != 0){
				tangent = normalize(vec3(model * vec4(tangent,0)));
				bitangent = cross(normal, tangent) * tangentIn.w;
			}
		}
	}
}