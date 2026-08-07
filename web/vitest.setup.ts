/*
 * Enough of a browser for Pixi to be built in, and no more.
 *
 * technical.md §10.4 asks for two effects tiers that are "correct from the first
 * scene", and the only way to hold that claim is to build both of them in a
 * test. Pixi's display objects are plain objects in Node — which is what lets
 * `scene.test.ts` walk a character across a shipped level with no GPU — but a
 * **filter** is not: constructing one asks the environment for a canvas so it
 * can find out what shader precision this machine supports.
 *
 * Pixi's own answer to that is `DOMAdapter`, which exists precisely so it can
 * run somewhere that is not a browser. The adapter below says "there is a
 * canvas, and it has no WebGL context", which is true here, and Pixi falls back
 * to `mediump` and carries on constructing the filter object. Nothing is
 * rendered in any test, so nothing downstream of that answer is exercised.
 *
 * This is deliberately not jsdom. A whole fake DOM would be a much larger claim
 * than the one being made, and the moment a test could *almost* render, someone
 * would write one that almost did.
 */

import { DOMAdapter } from 'pixi.js'

DOMAdapter.set({
  createCanvas: (width = 0, height = 0) =>
    ({
      width,
      height,
      // No WebGL here. Pixi checks for null and settles for medium precision.
      getContext: () => null,
    }) as unknown as ReturnType<(typeof DOMAdapter.get)['prototype']['createCanvas']>,
  createImage: () => ({}) as never,
  getCanvasRenderingContext2D: () => ({}) as never,
  getWebGLRenderingContext: () => ({}) as never,
  getNavigator: () => ({ userAgent: 'node', gpu: null }),
  getBaseUrl: () => '/',
  getFontFaceSet: () => null,
  fetch: () => Promise.reject(new Error('a test tried to fetch something')),
  parseXML: () => {
    throw new Error('a test tried to parse XML')
  },
})
