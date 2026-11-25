;;error:3:33-34:unknown symbol
(defcolumns (X :i16) (one :i1 :constant 1))
(deflookup test ((one X)) ((one Y)))

(module m1)
(defcolumns (Y :i16))
