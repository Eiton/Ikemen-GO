struct Light
{
    vec3 direction;
    float range;

    vec3 color;
    float intensity;

    vec3 position;
    float innerConeCos;

    float outerConeCos;
    int type;
};

uniform sampler2D tex;
uniform sampler2D normalMap;
uniform sampler2D metallicRoughnessMap;
uniform sampler2D ambientOcclusionMap;
uniform sampler2D shadowMap[8];
uniform samplerCube shadowCubeMap[8];

uniform vec3 cameraPosition;

uniform vec4 baseColorFactor;
uniform vec2 metallicRoughness;
uniform float ambientOcclusion;
uniform float farPlane;
uniform bool unlit;

uniform Light lights[4];


uniform vec3 add, mult;
uniform float gray, hue;
uniform bool useTexture;
uniform bool useNormalMap;
uniform bool useMetallicRoughnessMap;
uniform bool neg;
uniform bool enableAlpha;
uniform float alphaThreshold;

varying vec2 texcoord;
varying vec4 vColor;
varying vec3 normal;
varying vec3 tangent;
varying vec3 bitangent;
varying vec3 worldSpacePos;
varying vec4 lightSpacePos[8];

const float PI = 3.14159265358979;
const int LightType_Directional = 0;
const int LightType_Point = 1;
const int LightType_Spot = 2;

float clampedDot(vec3 x, vec3 y)
{
    return clamp(dot(x, y), 0.0, 1.0);
}

float ShadowCalculation(int index, vec4 lightSpacePos)
{
    // perform perspective divide
    vec3 projCoords = lightSpacePos.xyz / lightSpacePos.w;
    // transform to [0,1] range
    projCoords = projCoords * 0.5 + 0.5;
    // get closest depth value from light's perspective (using [0,1] range fragPosLight as coords)
    float closestDepth = texture2D(shadowMap[index], projCoords.xy).r; 
    // get depth of current fragment from light's perspective
    float currentDepth = projCoords.z;
    // check whether current frag pos is in shadow
    float shadow = closestDepth-currentDepth > -0.02  ? 1.0 : 0;

    return shadow;
}

float PointLightShadowCalculation(int index, vec3 pointToLight)
{
    float closestDepth = textureCube(shadowCubeMap[index], -pointToLight).r;
    // it is currently in linear range between [0,1]. Re-transform back to original value
    closestDepth *= farPlane;
    // now get current linear depth as the length between the fragment and light position
    float currentDepth = length(pointToLight);
    float shadow = currentDepth-closestDepth < 0.02  ? 1.0 : 0;

    return shadow;
}

vec3 getNormal()
{
    vec2 uv_dx = dFdx(texcoord);
    vec2 uv_dy = dFdy(texcoord);
    if (length(uv_dx) <= 1e-2) {
      uv_dx = vec2(1.0, 0.0);
    }

    if (length(uv_dy) <= 1e-2) {
      uv_dy = vec2(0.0, 1.0);
    }
    vec3 t_ = (uv_dy.t * dFdx(worldSpacePos) - uv_dx.t * dFdy(worldSpacePos)) /
        (uv_dx.s * uv_dy.t - uv_dy.s * uv_dx.t);
    vec3 n, t, b, ng;
    if(normal.x+normal.y+normal.z != 0){
        if(tangent.x+tangent.y+tangent.z != 0){
            t = normalize(tangent);
            b = normalize(bitangent);
            ng = normalize(normal);
        }else{
            ng = normalize(normal);
            t = normalize(t_ - ng * dot(ng, t_));
            b = cross(ng, t);
        }
    }else{
        ng = normalize(cross(dFdx(worldSpacePos), dFdy(worldSpacePos)));
        t = normalize(t_ - ng * dot(ng, t_));
        b = cross(ng, t);
    }
    if (gl_FrontFacing == false)
    {
        t *= -1.0;
        b *= -1.0;
        ng *= -1.0;
    }
    if(useNormalMap){
        return normalize(mat3(t, b, ng) * normalize(texture2D(normalMap, texcoord).xyz * 2.0 - vec3(1.0)));
        //return texture2D(normalMap, texcoord).xyz;
    }else{
        return ng;
    }
}

// https://github.com/KhronosGroup/glTF/blob/master/extensions/2.0/Khronos/KHR_lights_punctual/README.md#range-property
float getRangeAttenuation(float range, float distance)
{
    if (range <= 0.0)
    {
        // negative range means unlimited
        return 1.0 / pow(distance, 2.0);
    }
    return max(min(1.0 - pow(distance / range, 4.0), 1.0), 0.0) / pow(distance, 2.0);
}


// https://github.com/KhronosGroup/glTF/blob/master/extensions/2.0/Khronos/KHR_lights_punctual/README.md#inner-and-outer-cone-angles
float getSpotAttenuation(vec3 pointToLight, vec3 spotDirection, float outerConeCos, float innerConeCos)
{
    float actualCos = dot(normalize(spotDirection), normalize(-pointToLight));
    if (actualCos > outerConeCos)
    {
        if (actualCos < innerConeCos)
        {
            float angularAttenuation = (actualCos - outerConeCos) / (innerConeCos - outerConeCos);
            return angularAttenuation * angularAttenuation;
        }
        return 1.0;
    }
    return 0.0;
}
vec3 getLighIntensity(Light light, vec3 pointToLight)
{
    float rangeAttenuation = 1.0;
    float spotAttenuation = 1.0;

    if (light.type != LightType_Directional)
    {
        rangeAttenuation = getRangeAttenuation(light.range, length(pointToLight));
    }
    if (light.type == LightType_Spot)
    {
        spotAttenuation = getSpotAttenuation(pointToLight, light.direction, light.outerConeCos, light.innerConeCos);
    }

    return rangeAttenuation * spotAttenuation * light.intensity * light.color;
}
vec3 F_Schlick(vec3 f0, vec3 f90, float VdotH)
{
    return f0 + (f90 - f0) * pow(clamp(1.0 - VdotH, 0.0, 1.0), 5.0);
}
// Smith Joint GGX
// Note: Vis = G / (4 * NdotL * NdotV)
// see Eric Heitz. 2014. Understanding the Masking-Shadowing Function in Microfacet-Based BRDFs. Journal of Computer Graphics Techniques, 3
// see Real-Time Rendering. Page 331 to 336.
// see https://google.github.io/filament/Filament.md.html#materialsystem/specularbrdf/geometricshadowing(specularg)
float V_GGX(float NdotL, float NdotV, float alphaRoughness)
{
    float alphaRoughnessSq = alphaRoughness * alphaRoughness;

    float GGXV = NdotL * sqrt(NdotV * NdotV * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);
    float GGXL = NdotV * sqrt(NdotL * NdotL * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);

    float GGX = GGXV + GGXL;
    if (GGX > 0.0)
    {
        return 0.5 / GGX;
    }
    return 0.0;
}

// The following equation(s) model the distribution of microfacet normals across the area being drawn (aka D())
// Implementation from "Average Irregularity Representation of a Roughened Surface for Ray Reflection" by T. S. Trowbridge, and K. P. Reitz
// Follows the distribution function recommended in the SIGGRAPH 2013 course notes from EPIC Games [1], Equation 3.
float D_GGX(float NdotH, float alphaRoughness)
{
    float alphaRoughnessSq = alphaRoughness * alphaRoughness;
    float f = (NdotH * NdotH) * (alphaRoughnessSq - 1.0) + 1.0;
    return alphaRoughnessSq / (PI * f * f);
}
vec3 BRDF_lambertian(vec3 f0, vec3 f90, vec3 diffuseColor, float specularWeight, float VdotH)
{
    // see https://seblagarde.wordpress.com/2012/01/08/pi-or-not-to-pi-in-game-lighting-equation/
    return (1.0 - specularWeight * F_Schlick(f0, f90, VdotH)) * (diffuseColor / PI);
}
vec3 BRDF_specularGGX(vec3 f0, vec3 f90, float alphaRoughness, float specularWeight, float VdotH, float NdotL, float NdotV, float NdotH)
{
    vec3 F = F_Schlick(f0, f90, VdotH);
    float Vis = V_GGX(NdotL, NdotV, alphaRoughness);
    float D = D_GGX(NdotH, alphaRoughness);

    return specularWeight * F * Vis * D;
}
vec3 pbr(vec3 worldSpacePos,vec3 v,vec3 n,vec3 albedo,float metallic,float roughness,float ao,Light lights[4]){
	vec3 f0 = vec3(0.04)+(albedo-vec3(0.04))*metallic;
    vec3 f90 = vec3(1.0);
    float ior = 1.5;
    float specularWeight = 1.0;
    vec3 f_specular = vec3(0.0);
    vec3 f_diffuse = vec3(0.0);
    vec3 c_diff = albedo*(1-metallic);
    //mix(info.baseColor.rgb,  vec3(0), info.metallic);

	for(int i = 0; i < 8; ++i) 
    {
        if(lights[i].color.r+lights[i].color.g+lights[i].color.b > 0){
            vec3 pointToLight = vec3(0);
            if(lights[i].type == LightType_Directional){
                pointToLight = -lights[i].direction;
            }else{
                pointToLight = lights[i].position - worldSpacePos;
            }
            vec3 l = normalize(pointToLight);
            vec3 h = normalize(l + v);
            float NdotL = clampedDot(n, l);
            float NdotV = clampedDot(n, v);
            float NdotH = clampedDot(n, h);
            //float LdotH = clampedDot(l, h);
            float VdotH = clampedDot(v, h);
            if (NdotL > 0.0 || NdotV > 0.0){
                vec3 intensity = getLighIntensity(lights[i], pointToLight);
                vec3 l_diffuse = vec3(0.0);
                vec3 l_specular = vec3(0.0);
                //l_diffuse += 0.1*intensity * NdotL;
                l_diffuse += intensity * NdotL *  BRDF_lambertian(f0, f90, c_diff, specularWeight, VdotH);
                l_specular += intensity * NdotL * BRDF_specularGGX(f0, f90, roughness*roughness, specularWeight, VdotH, NdotL, NdotV, NdotH);
                float shadow = 1;
                if(lights[i].type == LightType_Point){
                    shadow = PointLightShadowCalculation(i,pointToLight);
                }else{
                    shadow = ShadowCalculation(i,lightSpacePos[i]);
                }
                f_diffuse += l_diffuse * shadow;//
                f_specular += l_specular * shadow;
            }
        }
    }   

    vec3 color = clamp(f_diffuse+f_specular,0,1);
    //color += albedo*0.2;
    
    //color = color / (color + vec3(1.0));
    //color = pow(color, vec3(1.0/2.2));
	return color;
}

vec3 hue_shift(vec3 color, float dhue) {
	float s = sin(dhue);
	float c = cos(dhue);
	return (color * c) + (color * s) * mat3(
		vec3(0.167444, 0.329213, -0.496657),
		vec3(-0.327948, 0.035669, 0.292279),
		vec3(1.250268, -1.047561, -0.202707)
	) + dot(vec3(0.299, 0.587, 0.114), color) * (1.0 - c);
}
void main(void) {
    gl_FragColor = vec4(1.0);
	if(useTexture){
		gl_FragColor = texture2D(tex, texcoord);
        gl_FragColor.rgb = pow(gl_FragColor.rgb,vec3(2.2));
	}
    gl_FragColor *= baseColorFactor;
	gl_FragColor *= vColor;
    if(!unlit){
        vec3 normalF = normal;
        if(useNormalMap){
            normalF = getNormal();
        }
        vec2 metallicRoughnessF = metallicRoughness;
        if(useMetallicRoughnessMap){
            metallicRoughnessF = texture2D(metallicRoughnessMap, texcoord).rg;
        }
        gl_FragColor.rgb = pbr(worldSpacePos,normalize(cameraPosition - worldSpacePos),normalize(normalF),gl_FragColor.rgb,metallicRoughnessF[0],metallicRoughnessF[1],ambientOcclusion,lights);
        //gl_FragColor.rgb *= ShadowCalculation(lightSpacePos);
    }
    gl_FragColor.rgb *= vColor.a;
    gl_FragColor.rgb = pow(gl_FragColor.rgb, vec3(1.0/2.2));
    //gl_FragColor.rgb = vec3(textureCube(shadowCubeMap[0], -(lights[0].position - worldSpacePos)).r)*12;
    //gl_FragColor.rgb = texture2D(shadowMap[0], lightSpacePos[0].xy/lightSpacePos[0].w*0.5+vec2(0.5)).rgb;
    //gl_FragColor.rgb = vec3(lightSpacePos[0].z/lightSpacePos[0].w*0.5+0.5);
    //normalF = normalize(normalF);
    //gl_FragColor.rgb = vec3((normalF.x+1)*0.5,(normalF.y+1)*0.5,(normalF.z+1)*0.5);
	if(!enableAlpha){
		if(gl_FragColor.a < alphaThreshold){
			discard;
		}else{
			gl_FragColor.a = 1;
		}
	}else if(gl_FragColor.a<=0.0){
		discard;
	}
	vec3 neg_base = vec3(1.0);
	neg_base *= gl_FragColor.a;
	if (hue != 0) {
		gl_FragColor.rgb = hue_shift(gl_FragColor.rgb,hue);			
	}
	if (neg) gl_FragColor.rgb = neg_base - gl_FragColor.rgb;
	gl_FragColor.rgb = mix(gl_FragColor.rgb, vec3((gl_FragColor.r + gl_FragColor.g + gl_FragColor.b) / 3.0), gray) + add*gl_FragColor.a;
	gl_FragColor.rgb *= mult;
}