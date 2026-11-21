export type Position = { x: number; y: number };

export type ViewportTransform = { x: number; y: number; k: number };

export enum CanvasNodeType {
  Image = "image",
  Text = "text",
  Drawing = "drawing",
  Script = "script",
  Skill = "skill",
  Config = "config",
  Video = "video",
  Audio = "audio",
  Frame = "frame",
  Panorama = "panorama",
  ColorGrade = "colorgrade",
  MediaConversion = "media-conversion",
  BatchTable = "batch-table",
}

export type PluginCanvasNodeType = string & { readonly __pluginCanvasNodeType?: unique symbol };
export type CanvasNodeTypeId = CanvasNodeType | PluginCanvasNodeType;
export type CanvasNodeStatus = "idle" | "success" | "loading" | "error";

export interface CanvasNodeMetadata {
  prompt?: string;
  model?: string;
  url?: string;
  thumbnailUrl?: string;
  status?: CanvasNodeStatus;
  errorDetails?: string;
  taskId?: string;
}

export interface CanvasNodeData {
  id: string;
  type: CanvasNodeTypeId;
  title: string;
  createdAt?: string;
  updatedAt?: string;
  position: Position;
  width: number;
  height: number;
  parentId?: string;
  metadata?: CanvasNodeMetadata;
}

export interface CanvasConnection {
  id: string;
  fromNodeId: string;
  toNodeId: string;
  fromHandleId?: string;
  toHandleId?: string;
  fromAnchorRatio?: number;
  toAnchorRatio?: number;
  relation?: "storyboard-output" | "storyboard-asset-reference" | "batch-output" | "batch-input";
  storyboardRowId?: string;
}
