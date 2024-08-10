uniform sampler2D tex;
uniform bool enableAlpha;
uniform bool useTexture;
uniform float alphaThreshold;
uniform vec4 baseColorFactor;
uniform bool isPointLight;
uniform vec3 lightPos;
uniform float farPlane;
varying vec4 FragPos;
varying vec4 vColor0;
varying vec2 texcoord0;
void main()
{
    vec4 color = baseColorFactor;
    if(useTexture){
        color = color * texture2D(tex, texcoord0);
    }
    color *= vColor0;
    if((enableAlpha && color.a <= 0) || (color.a < alphaThreshold)){
        discard;
    }
    if(isPointLight){
        float lightDistance = length(FragPos.xyz - lightPos);
    
        lightDistance = lightDistance / farPlane;
        
        gl_FragDepth = lightDistance;
    }else{
        gl_FragDepth = gl_FragCoord.z/gl_FragCoord.w;
    }
}