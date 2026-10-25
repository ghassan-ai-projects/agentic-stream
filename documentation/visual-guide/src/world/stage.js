import * as THREE from 'three';
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js';
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js';
import { rng } from '../util.js';
import { R } from './layout.js';

const groundShader = {
  vertexShader: `varying vec3 vW; void main(){ vec4 w = modelMatrix*vec4(position,1.0); vW = w.xyz; gl_Position = projectionMatrix*viewMatrix*w; }`,
  fragmentShader: `
    varying vec3 vW; uniform float uT; uniform float uR;
    float grid(vec2 p, float s, float w){ vec2 q = p/s; vec2 g = abs(fract(q-0.5)-0.5)/fwidth(q); return 1.0 - min(min(g.x,g.y)/w, 1.0); }
    void main(){
      float r = length(vW.xz);
      vec3 base = mix(vec3(0.030,0.048,0.100), vec3(0.008,0.013,0.028), smoothstep(0.0, 130.0, r));
      float fade = 1.0 - smoothstep(26.0, 120.0, r);
      float g = grid(vW.xz, 4.0, 1.1)*0.035 + grid(vW.xz, 20.0, 1.4)*0.09;
      vec3 col = base + vec3(0.18,0.38,0.62)*g*fade;
      float band = smoothstep(2.2, 0.0, abs(abs(r-uR)-5.2));
      col += vec3(0.04,0.10,0.20)*band*0.55;
      float line = smoothstep(0.22, 0.0, abs(r-uR));
      col += vec3(0.20,0.55,0.85)*line*0.30;
      float inner = smoothstep(0.25, 0.0, abs(r-(uR-5.2)))+smoothstep(0.25, 0.0, abs(r-(uR+5.2)));
      col += vec3(0.12,0.28,0.5)*inner*0.30;
      gl_FragColor = vec4(col, 1.0);
    }`,
};

export function createStage(canvas) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: 'high-performance' });
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.05;
  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0x03060d);
  scene.fog = new THREE.FogExp2(0x050a16, 0.0065);
  const camera = new THREE.PerspectiveCamera(42, 16 / 9, 0.5, 700);

  scene.add(new THREE.HemisphereLight(0x7f95d4, 0x0a0f1e, 1.15));
  const key = new THREE.DirectionalLight(0xcfe0ff, 2.4);
  key.position.set(40, 80, 60);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0x8b6cf6, 1.3);
  rim.position.set(-50, 30, -40);
  scene.add(rim);

  const ground = new THREE.Mesh(new THREE.CircleGeometry(190, 96), new THREE.ShaderMaterial({ ...groundShader, uniforms: { uT: { value: 0 }, uR: { value: R } } }));
  ground.rotation.x = -Math.PI / 2;
  ground.position.y = -0.05;
  scene.add(ground);

  const dust = makeDust();
  scene.add(dust);

  const composer = new EffectComposer(renderer, new THREE.WebGLRenderTarget(2, 2, { type: THREE.HalfFloatType, samples: 4 }));
  composer.addPass(new RenderPass(scene, camera));
  const bloom = new UnrealBloomPass(new THREE.Vector2(2, 2), 0.30, 0.45, 0.95);
  composer.addPass(bloom);
  composer.addPass(new OutputPass());

  function resize(w, h, dpr = Math.min(devicePixelRatio || 1, 2)) {
    renderer.setPixelRatio(dpr);
    renderer.setSize(w, h, false);
    composer.setPixelRatio(dpr);
    composer.setSize(w, h);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
  }

  function frame(T) {
    ground.material.uniforms.uT.value = T;
    dust.material.uniforms.uT.value = T;
    composer.render();
  }

  return { renderer, scene, camera, composer, bloom, resize, frame };
}

function makeDust() {
  const rand = rng(7);
  const n = 900;
  const pos = new Float32Array(n * 3);
  const seed = new Float32Array(n);
  for (let i = 0; i < n; i++) {
    const a = rand() * Math.PI * 2;
    const r = 8 + Math.sqrt(rand()) * 110;
    pos.set([Math.sin(a) * r, 0.5 + rand() * 38, Math.cos(a) * r], i * 3);
    seed[i] = rand() * 100;
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  g.setAttribute('seed', new THREE.BufferAttribute(seed, 1));
  const m = new THREE.ShaderMaterial({
    transparent: true, depthWrite: false, blending: THREE.AdditiveBlending,
    uniforms: { uT: { value: 0 } },
    vertexShader: `attribute float seed; uniform float uT; varying float vA;
      void main(){ vec3 p = position; p.y += sin(uT*0.22+seed)*1.6; p.x += sin(uT*0.13+seed*1.7)*1.4; p.z += cos(uT*0.11+seed*2.3)*1.4;
        vec4 mv = modelViewMatrix*vec4(p,1.0); gl_Position = projectionMatrix*mv; gl_PointSize = clamp(60.0/(-mv.z), 1.0, 3.2); vA = 0.25+0.35*fract(seed); }`,
    fragmentShader: `varying float vA; void main(){ vec2 c = gl_PointCoord-0.5; float d = smoothstep(0.5,0.0,length(c)); gl_FragColor = vec4(vec3(0.45,0.65,1.0), d*vA); }`,
  });
  return new THREE.Points(g, m);
}
