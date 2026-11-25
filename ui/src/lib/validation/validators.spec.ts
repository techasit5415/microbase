import type { FieldDef } from '../api/types';
import { expect, test } from 'vitest';
import { buildAndValidateRecordPayload as build, validateCollectionForm as validate } from './validators';
test('rejects duplicate fields and unknown index columns',()=>{
 expect(validate('posts',[{name:'title',type:'text'},{name:'title',type:'text'}],[],false)).toContain('Duplicate');
 expect(validate('posts',[],[{name:'idx_missing',columns:['missing']}],false)).toContain('unknown column');
});
test('rejects non-finite numbers before JSON converts them to null',()=>{expect(build([{name:'n',type:'number'}],{n:'Infinity'},{}).error).toBeDefined()});
test('parses multi-select arrays and validates all choices',()=>{
 const fields: FieldDef[]=[{name:'tags',type:'select',values:['a','b'],max_select:2}];
 expect(build(fields,{tags:'["a","b"]'},{}).payload).toEqual({tags:['a','b']});
 expect(build(fields,{tags:'["invalid"]'},{}).error).toBeDefined();
});
test('validates required, JSON, vectors and booleans',()=>{
 expect(build([{name:'x',type:'text',required:true}],{},{}).error).toBeDefined();
 expect(build([{name:'x',type:'json'}],{x:'{'},{}).error).toBeDefined();
 expect(build([{name:'x',type:'vector',dimension:2}],{x:'[1]'},{}).error).toBeDefined();
 expect(build([{name:'enabled',type:'boolean'}],{}, {enabled:false}).payload).toEqual({enabled:false});
});
