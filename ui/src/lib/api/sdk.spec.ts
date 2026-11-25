import { afterEach, expect, test, vi } from 'vitest';
import { Microbase, AuthStore } from '../../../../sdk/src/index';
afterEach(() => vi.unstubAllGlobals());
test('SDK preserves a record with a data field', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({id:'abc',data:{value:1}}))));
 const client = new Microbase('http://localhost',new AuthStore());
 expect(await client.collection('posts').getOne('abc')).toEqual({id:'abc',data:{value:1}});
});
test('SDK reports backend error code and message', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({code:'FORBIDDEN',message:'access denied'}),{status:403})));
 const client = new Microbase('http://localhost',new AuthStore());
 await expect(client.collection('posts').getList()).rejects.toMatchObject({status:403,code:'FORBIDDEN',message:'access denied'});
});
test('SDK receives named realtime events and closes subscription', () => {
 const listeners = new Map<string, EventListener>();
 const close = vi.fn();
 vi.stubGlobal('EventSource', class { addEventListener(name:string,fn:EventListener) {listeners.set(name,fn)} close=close; });
 const callback = vi.fn();
 const unsubscribe = new Microbase('http://localhost',new AuthStore()).collection('posts').subscribe(callback);
 listeners.get('create')?.({data:JSON.stringify({action:'create',record:{id:'abc'}})} as unknown as Event);
 expect(callback).toHaveBeenCalledWith({action:'create',record:{id:'abc'}});
 unsubscribe();
 expect(close).toHaveBeenCalledOnce();
});
