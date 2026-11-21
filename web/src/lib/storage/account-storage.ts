import localforage from "localforage";

const instances = new Map<string, LocalForage>();

export function getAccountStorage(userId: string | null | undefined, namespace = "canvas"): LocalForage {
  const scope = userId && userId.trim() !== "" ? userId.trim() : "anonymous";
  const key = `${namespace}:${scope}`;
  let store = instances.get(key);
  if (!store) {
    store = localforage.createInstance({
      name: "open-ai-canvas",
      storeName: `${namespace}_${scope}`,
    });
    instances.set(key, store);
  }
  return store;
}

export async function clearAccountStorage(userId: string | null | undefined, namespace = "canvas"): Promise<void> {
  const store = getAccountStorage(userId, namespace);
  await store.clear();
}
