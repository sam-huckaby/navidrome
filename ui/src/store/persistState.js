export const loadState = () => {
  try {
    const serializedState = localStorage.getItem('state')
    if (serializedState === null) {
      return undefined
    }
    const state = JSON.parse(serializedState)
    // A saved choice from the old theme picker must not survive the migration.
    if (state && typeof state === 'object') {
      delete state.theme
    }
    return state
  } catch (err) {
    return undefined
  }
}

export const saveState = (state) => {
  try {
    const serializedState = JSON.stringify(state)
    localStorage.setItem('state', serializedState)
  } catch (err) {
    // Ignore write errors
  }
}
