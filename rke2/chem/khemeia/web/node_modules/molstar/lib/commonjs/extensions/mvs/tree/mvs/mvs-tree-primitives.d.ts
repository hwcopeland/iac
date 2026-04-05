/**
 * Copyright (c) 2023-2025 mol* contributors, licensed under MIT, See LICENSE file for more info.
 *
 * @author David Sehnal <david.sehnal@gmail.com>
 * @author Adam Midlik <midlik@gmail.com>
 */
import { OptionalField, RequiredField } from '../generic/field-schema.js';
import { SimpleParamsSchema, UnionParamsSchema } from '../generic/params-schema.js';
import { ColorT, Vector3 } from './param-types.js';
export declare const MVSPrimitiveParams: UnionParamsSchema<"kind", {
    mesh: SimpleParamsSchema<{
        /** 3*n_vertices length array of floats with vertex position (x1, y1, z1, ...). */
        vertices: RequiredField<number[]>;
        /** 3*n_triangles length array of indices into vertices that form triangles (t1_1, t1_2, t1_3, ...). */
        indices: RequiredField<number[]>;
        /** Assign a number to each triangle to group them. If not specified, each triangle is considered a separate group (triangle i = group i). */
        triangle_groups: OptionalField<number[] | null>;
        /** Assign a color to each group. Where not assigned, uses `color`. */
        group_colors: OptionalField<{
            [x: number]: ColorT;
        }>;
        /** Assign a tooltip to each group. Where not assigned, uses `tooltip`. */
        group_tooltips: OptionalField<{
            [x: number]: string;
        }>;
        /** Color of the triangles and wireframe. Can be overwritten by `group_colors`. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
        /** Tooltip shown when hovering over the mesh. Can be overwritten by `group_tooltips`. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
        /** Determine whether to render triangles of the mesh. */
        show_triangles: OptionalField<boolean>;
        /** Determine whether to render wireframe of the mesh. */
        show_wireframe: OptionalField<boolean>;
        /** Wireframe line width (in screen-space units). */
        wireframe_width: OptionalField<number>;
        /** Wireframe color. If not specified, uses `group_colors`. */
        wireframe_color: OptionalField<ColorT | null>;
    }>;
    lines: SimpleParamsSchema<{
        /** 3*n_vertices length array of floats with vertex position (x1, y1, z1, ...). */
        vertices: RequiredField<number[]>;
        /** 2*n_lines length array of indices into vertices that form lines (l1_1, l1_2, ...). */
        indices: RequiredField<number[]>;
        /** Assign a number to each line to group them. If not specified, each line is considered a separate group (line i = group i). */
        line_groups: OptionalField<number[] | null>;
        /** Assign a color to each group. Where not assigned, uses `color`. */
        group_colors: OptionalField<{
            [x: number]: ColorT;
        }>;
        /** Assign a tooltip to each group. Where not assigned, uses `tooltip`. */
        group_tooltips: OptionalField<{
            [x: number]: string;
        }>;
        /** Assign a line width to each group. Where not assigned, uses `width`. */
        group_widths: OptionalField<{
            [x: number]: number;
        }>;
        /** Color of the lines. Can be overwritten by `group_colors`. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
        /** Tooltip shown when hovering over the lines. Can be overwritten by `group_tooltips`. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
        /** Line width (in screen-space units). Can be overwritten by `group_widths`. */
        width: OptionalField<number>;
    }>;
    tube: SimpleParamsSchema<{
        /** Tooltip to show when hovering over the tube. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
        /** Start point of the tube. */
        start: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** End point of the tube. */
        end: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Tube radius (in Angstroms). */
        radius: OptionalField<number>;
        /** Length of each dash and gap between dashes. If not specified (null), draw full line. */
        dash_length: OptionalField<number | null>;
        /** Color of the tube. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
    }>;
    arrow: SimpleParamsSchema<{
        /** Start point of the tube. */
        start: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** End point of the tube. */
        end: OptionalField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT | null>;
        /** If specified, the endpoint is computed as start + direction. */
        direction: OptionalField<Vector3 | null>;
        /** Length of the arrow. If unset, the distance between start and end is used. */
        length: OptionalField<number | null>;
        /** Draw a cap at the start of the arrow. */
        show_start_cap: OptionalField<boolean>;
        /** Length of the start cap. */
        start_cap_length: OptionalField<number | null>;
        /** Radius of the start cap. */
        start_cap_radius: OptionalField<number | null>;
        /** Draw an arrow at the end of the arrow. */
        show_end_cap: OptionalField<boolean>;
        /** Height of the arrow at the end. */
        end_cap_length: OptionalField<number | null>;
        /** Radius of the arrow at the end. */
        end_cap_radius: OptionalField<number | null>;
        /** Draw a tube connecting the start and end points. */
        show_tube: OptionalField<boolean>;
        /** Tube radius (in Angstroms). */
        tube_radius: OptionalField<number>;
        /** Length of each dash and gap between dashes. If not specified (null), draw full line. */
        tube_dash_length: OptionalField<number | null>;
        /** Color of the tube. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
        /** Tooltip to show when hovering over the tube. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
    }>;
    distance_measurement: SimpleParamsSchema<{
        /** Template used to construct the label. Use {{distance}} as placeholder for the distance. */
        label_template: OptionalField<string>;
        /** Size of the label (text height in Angstroms). If not specified, size will be relative to the distance (see label_auto_size_scale, label_auto_size_min). */
        label_size: OptionalField<number | null>;
        /** Scaling factor for relative size. */
        label_auto_size_scale: OptionalField<number>;
        /** Minimum size for relative size. */
        label_auto_size_min: OptionalField<number>;
        /** Color of the label. If not specified, uses the parent primitives group `label_color`. */
        label_color: OptionalField<ColorT | null>;
        /** Start point of the tube. */
        start: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** End point of the tube. */
        end: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Tube radius (in Angstroms). */
        radius: OptionalField<number>;
        /** Length of each dash and gap between dashes. If not specified (null), draw full line. */
        dash_length: OptionalField<number | null>;
        /** Color of the tube. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
    }>;
    angle_measurement: SimpleParamsSchema<{
        /** Point A. */
        a: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Point B. */
        b: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Point C. */
        c: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Template used to construct the label. Use {{angle}} as placeholder for the angle in radians. */
        label_template: OptionalField<string>;
        /** Size of the label (text height in Angstroms). If not specified, size will be relative to the distance (see label_auto_size_scale, label_auto_size_min). */
        label_size: OptionalField<number | null>;
        /** Scaling factor for relative size. */
        label_auto_size_scale: OptionalField<number>;
        /** Minimum size for relative size. */
        label_auto_size_min: OptionalField<number>;
        /** Color of the label. If not specified, uses the parent primitives group `label_color`. */
        label_color: OptionalField<ColorT | null>;
        /** Draw vectors between (a, b) and (b, c). */
        show_vector: OptionalField<boolean>;
        /** Color of the vectors. */
        vector_color: OptionalField<ColorT | null>;
        /** Radius of the vectors. */
        vector_radius: OptionalField<number>;
        /** Draw a filled circle section representing the angle. */
        show_section: OptionalField<boolean>;
        /** Color of the angle section. If not specified, the primitives group color is used. */
        section_color: OptionalField<ColorT | null>;
        /** Radius of the angle section. In angstroms. */
        section_radius: OptionalField<number | null>;
        /** Factor to scale the radius of the angle section. Ignored if section_radius is set. */
        section_radius_scale: OptionalField<number>;
    }>;
    label: SimpleParamsSchema<{
        /** Position of this label. */
        position: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** The label. */
        text: RequiredField<string>;
        /** Size of the label (text height in Angstroms). */
        label_size: OptionalField<number>;
        /** Color of the label. If not specified, uses the parent primitives group `label_color`. */
        label_color: OptionalField<ColorT | null>;
        /** Camera-facing offset to prevent overlap with geometry. */
        label_offset: OptionalField<number>;
    }>;
    ellipse: SimpleParamsSchema<{
        /** Color of the primitive. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
        /** If true, ignores radius_minor/magnitude of the minor axis */
        as_circle: OptionalField<boolean>;
        /** ellipse center. */
        center: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Major axis of this ellipse. */
        major_axis: OptionalField<Vector3 | null>;
        /** Minor axis of this ellipse. */
        minor_axis: OptionalField<Vector3 | null>;
        /** Major axis endpoint. If specified, overrides major axis to be major_axis_endpoint - center. */
        major_axis_endpoint: OptionalField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT | null>;
        /** Minor axis endpoint. If specified, overrides minor axis to be minor_axis_endpoint - center. */
        minor_axis_endpoint: OptionalField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT | null>;
        /** Radius of the major axis. If unset, the length of the major axis is used. */
        radius_major: OptionalField<number | null>;
        /** Radius of the minor axis. If unset, the length of the minor axis is used. */
        radius_minor: OptionalField<number | null>;
        /** Start of the arc. In radians */
        theta_start: OptionalField<number>;
        /** End of the arc. In radians */
        theta_end: OptionalField<number>;
        /** Tooltip to show when hovering over the tube. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
    }>;
    ellipsoid: SimpleParamsSchema<{
        /** Color of the primitive. If not specified, uses the parent primitives group `color`. */
        color: OptionalField<ColorT | null>;
        /** Ellipsoid center. */
        center: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** Major axis of this ellipsoid. */
        major_axis: OptionalField<Vector3 | null>;
        /** Minor axis of this ellipsoid. */
        minor_axis: OptionalField<Vector3 | null>;
        /** Major axis endpoint. If specified, overrides major axis to be major_axis_endpoint - center. */
        major_axis_endpoint: OptionalField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT | null>;
        /** Minor axis endpoint. If specified, overrides minor axis to be minor_axis_endpoint - center. */
        minor_axis_endpoint: OptionalField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT | null>;
        /** Radii of the ellipsoid along each axis. */
        radius: OptionalField<number | Vector3 | null>;
        /** Added to the radii of the ellipsoid along each axis. */
        radius_extent: OptionalField<number | Vector3 | null>;
        /** Tooltip to show when hovering over the tube. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
    }>;
    box: SimpleParamsSchema<{
        /** The center of the box. */
        center: RequiredField<import("./param-types.js").ComponentExpressionT | Vector3 | import("./param-types.js").PrimitiveComponentExpressionT>;
        /** The width, the height, and the depth of the box. Added to the bounding box determined by the center. */
        extent: OptionalField<Vector3 | null>;
        /** Determine whether to render the faces of the box. */
        show_faces: OptionalField<boolean>;
        /** Color of the box faces. */
        face_color: OptionalField<ColorT | null>;
        /** Determine whether to render the edges of the box. */
        show_edges: OptionalField<boolean>;
        /** Radius of the box edges. In angstroms. */
        edge_radius: OptionalField<number>;
        /** Color of the box edges. */
        edge_color: OptionalField<ColorT | null>;
        /** Tooltip to show when hovering over the tube. If not specified, uses the parent primitives group `tooltip`. */
        tooltip: OptionalField<string | null>;
    }>;
}>;
