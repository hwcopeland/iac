import { Expression } from './expression.js';
export interface Container {
    source?: string;
    version: string;
    expression: Expression;
}
