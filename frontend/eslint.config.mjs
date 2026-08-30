import nextConfig from "eslint-config-next";

const config = [
  {
    ignores: [".next/**", "node_modules/**", "out/**"],
  },
  ...nextConfig,
  {
    rules: {
      // React Compiler lint rules produce false positives for standard patterns:
      // - set-state-in-effect: flags initialization data fetching in useEffect
      // - immutability: flags forward-declared async functions called in useEffect
      // - refs: flags ref sync patterns needed for stale closure avoidance
      // - pure-render: flags Date.now(), Math.random() etc. used in render for display
      // - purity: flags Date.now(), Math.random() in JSX expressions
      // - preserve-manual-memoization: flags existing useCallback/useMemo that compiler can't preserve
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/immutability": "off",
      "react-hooks/refs": "off",
      "react-hooks/pure-render": "off",
      "react-hooks/purity": "off",
      "react-hooks/preserve-manual-memoization": "off",
      "react-compiler/react-compiler": "off",
    },
  },
];

export default config;
