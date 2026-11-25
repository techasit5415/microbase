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
	presentable?: boolean;
	hidden?: boolean;
	help_text?: string;

	// Relation options
	collection_name?: string;
	cascade_delete?: boolean;

	// Select / File / Relation multiplicity
	max_select?: number;
	values?: string[];

	// File options
	mime_types?: string[];
	thumbs?: string[];
	max_size?: number;
	protected?: boolean;

	// Text / Number / Email / URL / Autodate options
	min?: number;
	max?: number;
	pattern?: string;
	only_int?: boolean;
	only_domains?: string[];
	except_domains?: string[];
	on_create?: boolean;
	on_update?: boolean;

	// Vector options
	dimension?: number;
}

export interface IndexDef {
	name: string;
	columns: string[];
	unique?: boolean;
}

export interface Collection {
	id: number;
	name: string;
	type?: 'auth' | 'base' | 'system';
	fields: FieldDef[];
	api_rules?: Record<string, string>;
	indexes?: IndexDef[];
	created_at: string;
	updated_at: string;
}

export type RecordRow = Record<string, unknown>;

export interface RecordListResponse {
	page: number;
	per_page: number;
	total_items: number;
	items: RecordRow[];
}

export interface ListResult<T = RecordRow> {
	page: number;
	per_page: number;
	total_items: number;
	items: T[];
}
