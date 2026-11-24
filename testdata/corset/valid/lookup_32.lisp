;; As lookup_01, but without selectors.
(defcolumns (X :i16) (Y :i16))
(deflookup test ((_ X)) ((_ Y)))
