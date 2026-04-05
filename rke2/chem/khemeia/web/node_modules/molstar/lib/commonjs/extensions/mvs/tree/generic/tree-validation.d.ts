import { PluginContext } from '../../../../mol-plugin/context.js';
import { Tree, TreeSchema } from './tree-schema.js';
/** Return `undefined` if a tree conforms to the given schema,
 * return validation issues (as a list of lines) if it does not conform.
 * If `options.requireAll`, all parameters (including optional) must have a value provided.
 * If `options.noExtra` is true, presence of any extra parameters is treated as an issue.
 * If `options.anyRoot` is true, the kind of the root node is not enforced.
 */
export declare function treeValidationIssues(schema: TreeSchema, tree: Tree, options?: {
    requireAll?: boolean;
    noExtra?: boolean;
    anyRoot?: boolean;
    parent?: string;
}): string[] | undefined;
/** Validate a tree against the given schema.
 * Do nothing if OK; print validation issues on console and throw an error is the tree does not conform.
 * Include `label` in the printed output. */
export declare function validateTree(schema: TreeSchema, tree: Tree, label: string, plugin: PluginContext): void;
/** Return documentation for a tree schema as plain text */
export declare function treeSchemaToString<S extends TreeSchema>(schema: S): string;
/** Return documentation for a tree schema as markdown text */
export declare function treeSchemaToMarkdown<S extends TreeSchema>(schema: S): string;
