export type FieldType =
	| 'text'
	| 'editor'
	| 'number'
	| 'integer'
	| 'float'
	| 'boolean'
	| 'email'
	| 'url'
	| 'datetime'
	| 'timestamptz'
	| 'autodate'
	| 'file'
	| 'relation'
	| 'select'
	| 'json'
	| 'geopoint'
	| 'vector';

export interface FieldDef {
	name: string;
	original_name?: string;
	type: FieldType;
	required?: boolean;
	unique?: boolean;
	dimension?: number;
}

export interface Collection {
	id: number;
	name: string;
	type?: 'auth' | 'base' | 'system';
	fields: FieldDef[];
	api_rules?: Record<string, string>;
	created_at: string;
	updated_at: string;
}

export type RecordRow = Record<string, unknown> & {
	id?: number;
	created_at?: string;
	updated_at?: string;
};

export interface ListResult {
	page: number;
	per_page: number;
	total_items: number;
	items: RecordRow[];
}
