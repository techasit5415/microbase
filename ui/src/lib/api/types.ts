export type FieldType =
	| 'text'
	| 'integer'
	| 'float'
	| 'boolean'
	| 'json'
	| 'timestamptz'
	| 'vector';

export interface FieldDef {
	name: string;
	type: FieldType;
	required?: boolean;
	unique?: boolean;
	dimension?: number;
}

export interface Collection {
	id: number;
	name: string;
	fields: FieldDef[];
	created_at: string;
	updated_at: string;
}

export type RecordRow = Record<string, unknown> & {
	id: number;
	created_at: string;
	updated_at: string;
};

export interface ListResult {
	page: number;
	per_page: number;
	total_items: number;
	items: RecordRow[];
}

export interface RealtimeEvent {
	action: 'create' | 'update' | 'delete';
	collection: string;
	record: unknown;
	timestamp: string;
}