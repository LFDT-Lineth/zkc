;;error:3:29-30:unknown symbol
(defcolumns (X :i16))
(deflookup test ((_ X)) ((_ Y)))

(module m1)
(defcolumns (Y :i16))
