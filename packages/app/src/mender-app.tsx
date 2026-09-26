import { motion } from "motion/react";
import { Route, Routes } from "react-router";

function FoundationScreen() {
  return (
    <main className="mx-auto flex min-h-screen max-w-5xl items-center px-6 py-16">
      <motion.section
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2 }}
        className="space-y-4"
      >
        <p className="text-sm font-medium tracking-wide uppercase">Mender</p>
        <h1 className="text-4xl font-semibold tracking-tight">
          Tool runtime and gateway for LLMs and agents
        </h1>
        <p className="max-w-2xl text-base leading-7 opacity-70">
          The repository foundation is active. Product capabilities will be
          introduced behind the documented domain and application boundaries.
        </p>
      </motion.section>
    </main>
  );
}

export function MenderApp() {
  return (
    <Routes>
      <Route path="*" element={<FoundationScreen />} />
    </Routes>
  );
}
