// Metro config for the stocker monorepo.
// core/ lives OUTSIDE app/, so its imports (e.g. "react" from core/usePoll.ts)
// would otherwise resolve up the tree and find the unrelated
// /Users/.../Workspace/react/node_modules. Pin the shared packages to
// app/node_modules and watch ../core so it can be bundled.
//
// Note: tsconfig.json also maps "react" -> ./node_modules/@types/react so the
// TYPE CHECKER doesn't find the parent tree's untyped react; the
// resolveRequest hook below runs before Expo's tsconfig-paths expansion and
// short-circuits that mapping for the bundler.
const path = require("path");
const { getDefaultConfig } = require("expo/metro-config");

const config = getDefaultConfig(__dirname);

const coreDir = path.resolve(__dirname, "../core");
const appModules = path.resolve(__dirname, "node_modules");

config.watchFolders = [coreDir];

// Fallback: force bare imports from files outside app/ to resolve against
// app/node_modules instead of walking up the directory tree.
config.resolver.extraNodeModules = new Proxy(
  {},
  { get: (_target, name) => path.join(appModules, name) },
);

// Runs before Expo's tsconfig-paths handling: pin the packages that must come
// from app/node_modules (never from a parent node_modules, and never from the
// types-only tsconfig "react" mapping).
const PINNED = new Set(["react", "react-native", "react-native-svg"]);
const upstream = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (PINNED.has(moduleName)) {
    return context.resolveRequest(context, path.join(appModules, moduleName), platform);
  }
  if (upstream) return upstream(context, moduleName, platform);
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;
