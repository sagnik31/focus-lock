// createSerialQueue returns a function that runs async tasks one at a time, in
// order. A failed task rejects for its caller and doesn't stop later ones.
export function createSerialQueue(): <T>(task: () => Promise<T>) => Promise<T> {
    let tail: Promise<unknown> = Promise.resolve();
    return task => {
        const result = tail.then(task);
        tail = result.catch(() => undefined);
        return result;
    };
}
