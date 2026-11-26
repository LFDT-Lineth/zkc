;;error:3:23-24:unknown symbol
(defcolumns (X :i16) (one :i1 :constant 1))
(deflookup test ((one Y)) ((one X)))

(module m1)
(defcolumns (Y :i16))
