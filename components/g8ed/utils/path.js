import path from 'path';

/**
 * Resolves the project root directory.
 * Priority:
 * 1. G8E_PROJECT_ROOT environment variable
 * 2. Fallback: walks up from current working directory until it finds the root
 *    (Assuming being called from within the repository structure)
 * 
 * @returns {string} The absolute path to the project root
 */
function resolveProjectRoot() {
  if (process.env.G8E_PROJECT_ROOT) {
    return path.resolve(process.env.G8E_PROJECT_ROOT);
  }

  // Current structure for g8ed is: <root>/components/g8ed
  // If we are in components/g8ed/services/platform/bootstrap_service.js,
  // we are 4 levels deep from root if we count from the file.
  // But usually cwd is components/g8ed when running the app.
  
  const cwd = process.cwd();
  
  // If we are in components/g8ed, parent.parent is root
  if (cwd.includes(path.join('components', 'g8ed'))) {
     // If we are exactly in components/g8ed, or a subdirectory
     let current = cwd;
     while (current !== path.parse(current).root) {
       if (path.basename(path.dirname(current)) === 'components' && path.basename(current) === 'g8ed') {
         return path.resolve(current, '..', '..');
       }
       current = path.dirname(current);
     }
  }

  // Generic fallback if we can't detect being in components/g8ed
  // Just assume we are 2 levels deep if in a component
  return path.resolve(cwd, '..', '..');
}

export {
  resolveProjectRoot
};
