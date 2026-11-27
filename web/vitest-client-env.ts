import type { Environment } from "vitest/runtime";

/**
 * A vitest environment that is plain node but transformed as client code (viteEnvironment "client"),
 * so vite-plugin-svelte compiles .svelte.ts modules for the browser and $effect runs under flushSync.
 */
const env = {
  name: "bv-node-client",
  viteEnvironment: "client",
  setup() {
    return { teardown() {} };
  },
} satisfies Environment;

export default env;
